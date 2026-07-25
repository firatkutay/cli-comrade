#!/usr/bin/env node
'use strict';

// A minimal, dependency-free local npm registry for test-smoke.sh --
// stdlib `node:http`/`node:fs`/`node:child_process` ONLY, no
// package.json/lockfile of its own. It serves exactly two things, built
// from an already-assembled npm/packages/-style directory (one
// subdirectory per package, each a real, `npm pack`-able package):
//
//   GET /<name>              -> a hand-built "packument" (the JSON npm's
//                               installer reads to pick a version and
//                               find the tarball URL)
//   GET /<name>/-/<file>.tgz -> the real tarball bytes, produced by
//                               shelling out to the ALREADY-PRESENT `npm
//                               pack` (so shasum/integrity are computed
//                               by npm itself, not reimplemented here)
//
// There is no publish/write endpoint, no auth, and no proxy/uplink of any
// kind -- this process cannot reach the real npm registry even by
// accident, because the code to do so simply does not exist here. This
// is intentionally narrower than a general-purpose local registry
// (verdaccio, etc.): it exists to answer exactly one question --
// "does `npm install -g cli-comrade` against a registry that only knows
// about our own packages resolve the optionalDependency correctly and
// link the dispatcher?" -- see the reviewed incident this replaces:
// npm/test/test-smoke.sh's own file header.
//
// Usage: node local-registry.js <packages-dir>
// Prints exactly one line, "PORT=<port>", to stdout once listening.

const fs = require('fs');
const path = require('path');
const http = require('http');
const os = require('os');
const { execFileSync } = require('child_process');

const packagesDir = process.argv[2];
if (!packagesDir) {
  console.error('local-registry.js: usage: node local-registry.js <packages-dir>');
  process.exit(1);
}

const tarballDir = fs.mkdtempSync(path.join(os.tmpdir(), 'local-registry-tarballs-'));

// name -> { packument, tarballPath, tarballRoute }
const registry = new Map();

for (const entry of fs.readdirSync(packagesDir, { withFileTypes: true })) {
  if (!entry.isDirectory()) continue;
  const pkgDir = path.join(packagesDir, entry.name);
  const pkgJsonPath = path.join(pkgDir, 'package.json');
  if (!fs.existsSync(pkgJsonPath)) continue;

  const manifest = JSON.parse(fs.readFileSync(pkgJsonPath, 'utf8'));

  // `npm pack --json` returns the real filename/shasum/integrity for the
  // tarball it just built -- computed by npm itself, not reimplemented
  // here, so this cannot silently drift from what a real publish would
  // compute.
  const packOutput = execFileSync(
    'npm',
    ['pack', '--json', '--pack-destination', tarballDir, pkgDir],
    { encoding: 'utf8' }
  );
  const [packed] = JSON.parse(packOutput);

  // `tarballRoute` is the DECODED form (matches `requestedPath` below, which
  // is also decoded) -- used only for our own internal route matching.
  // `tarballUrlPath` is the properly percent-encoded form that must appear
  // in the actual URL text handed to the npm client (an `@scope/name`'s
  // literal "/" would otherwise be indistinguishable from a path
  // separator).
  const tarballRoute = `/${manifest.name}/-/${packed.filename}`;
  const tarballUrlPath = `/${encodeURIComponent(manifest.name)}/-/${packed.filename}`;
  const packument = {
    name: manifest.name,
    'dist-tags': { latest: manifest.version },
    versions: {
      [manifest.version]: {
        ...manifest,
        dist: {
          tarball: `__BASE_URL__${tarballUrlPath}`,
          shasum: packed.shasum,
          integrity: packed.integrity,
        },
      },
    },
  };

  registry.set(manifest.name, {
    packument,
    tarballPath: path.join(tarballDir, packed.filename),
    tarballRoute,
  });
}

const server = http.createServer((req, res) => {
  const requestedPath = decodeURIComponent(req.url.split('?')[0]);

  for (const { packument, tarballPath, tarballRoute } of registry.values()) {
    if (requestedPath === tarballRoute) {
      res.writeHead(200, { 'Content-Type': 'application/octet-stream' });
      fs.createReadStream(tarballPath).pipe(res);
      return;
    }
    if (requestedPath === `/${packument.name}`) {
      const baseUrl = `http://${req.headers.host}`;
      const body = JSON.stringify(packument).replaceAll('__BASE_URL__', baseUrl);
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(body);
      return;
    }
  }

  res.writeHead(404, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ error: `not found: ${requestedPath}` }));
});

server.listen(0, '127.0.0.1', () => {
  console.log(`PORT=${server.address().port}`);
});

// `exit` fires synchronously on every termination path (including the
// process.exit(0) calls below), so a single handler here is enough to
// remove the tarball staging directory regardless of how this process
// ends.
process.on('exit', () => fs.rmSync(tarballDir, { recursive: true, force: true }));
process.on('SIGTERM', () => process.exit(0));
process.on('SIGINT', () => process.exit(0));
