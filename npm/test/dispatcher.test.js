'use strict';

// Plain assertion-script test (no test-framework dependency) -- run via
// `node dispatcher.test.js`; throws (non-zero exit) on any failed
// assertion. Aggregated by npm/test/run-node-tests.sh.

const assert = require('assert/strict');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawnSync } = require('child_process');

const DISPATCHER_DIR = path.join(__dirname, '..', 'main', 'bin');
const DISPATCHER_PATH = path.join(DISPATCHER_DIR, 'comrade.js');
const comrade = require(path.join(DISPATCHER_DIR, 'comrade.js'));

// ===========================================================================
// 1. Unsupported platform -- pure-function check (resolveBinaryPath),
//    exercising process.platform/process.arch combinations this machine
//    cannot naturally have, without spawning a child process.
// ===========================================================================

(function testUnsupportedPlatformMessage() {
  const originalPlatform = Object.getOwnPropertyDescriptor(process, 'platform');
  const originalArch = Object.getOwnPropertyDescriptor(process, 'arch');

  Object.defineProperty(process, 'platform', { value: 'sunos', configurable: true });
  Object.defineProperty(process, 'arch', { value: 'x64', configurable: true });

  try {
    const resolved = comrade.resolveBinaryPath();
    assert.equal(resolved.binaryPath, undefined, 'an unsupported platform must not resolve a binary path');
    assert.match(
      resolved.error,
      /no prebuilt binary available for this platform \("sunos-x64"\)/,
      'error message must name the exact unsupported platform-arch pair'
    );
    assert.match(
      resolved.error,
      /not part of the npm distribution matrix/,
      'error message must explain why (unsupported combination, not just "missing")'
    );
    assert.match(resolved.error, /Supported npm platforms:/, 'error message must list supported platforms');
    assert.match(resolved.error, /brew install/, 'error message must point at an alternative install channel');
    assert.match(resolved.error, /winget install/, 'error message must point at winget as an alternative');
  } finally {
    Object.defineProperty(process, 'platform', originalPlatform);
    Object.defineProperty(process, 'arch', originalArch);
  }
})();

(function testWindowsArm64ExplicitlyUnsupported() {
  // windows/arm64 is explicitly excluded from .goreleaser.yaml's build
  // matrix -- confirm the dispatcher treats it the same as any other
  // unsupported combination, not as a silently-tolerated gap.
  const originalPlatform = Object.getOwnPropertyDescriptor(process, 'platform');
  const originalArch = Object.getOwnPropertyDescriptor(process, 'arch');

  Object.defineProperty(process, 'platform', { value: 'win32', configurable: true });
  Object.defineProperty(process, 'arch', { value: 'arm64', configurable: true });

  try {
    const resolved = comrade.resolveBinaryPath();
    assert.match(resolved.error, /"win32-arm64"/);
  } finally {
    Object.defineProperty(process, 'platform', originalPlatform);
    Object.defineProperty(process, 'arch', originalArch);
  }
})();

// ===========================================================================
// 2. End-to-end: supported platform, but no matching optional dependency
//    installed -- the real state of a fresh checkout of this repo (no
//    node_modules at all). Must print the friendly message, not a raw
//    MODULE_NOT_FOUND stack, and exit non-zero.
// ===========================================================================

(function testMissingOptionalDependencyEndToEnd() {
  const result = spawnSync(process.execPath, [DISPATCHER_PATH, '--version'], {
    encoding: 'utf8',
    cwd: DISPATCHER_DIR,
  });

  assert.notEqual(result.status, 0, 'must exit non-zero when the platform package is not installed');
  assert.match(
    result.stderr,
    /no prebuilt binary available for this platform/,
    'must print the friendly message, not a raw stack trace'
  );
  assert.match(result.stderr, /is not installed/);
  assert.doesNotMatch(
    result.stderr,
    /MODULE_NOT_FOUND/,
    'must not leak the raw require.resolve MODULE_NOT_FOUND code to the user'
  );
  assert.doesNotMatch(
    result.stderr,
    /at Module\._resolveFilename/,
    'must not leak a raw Node internals stack trace to the user'
  );
})();

// ===========================================================================
// Fixture-backed tests: a fake platform package for THIS machine's actual
// platform/arch, so runBinary()'s exit-code/signal/argv forwarding can be
// exercised through a real spawnSync() round trip.
// ===========================================================================

function withFixture(fn) {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'comrade-dispatcher-test-'));
  try {
    const binDir = path.join(tmpRoot, 'bin');
    fs.mkdirSync(binDir, { recursive: true });
    fs.copyFileSync(path.join(DISPATCHER_DIR, 'comrade.js'), path.join(binDir, 'comrade.js'));
    fs.copyFileSync(path.join(DISPATCHER_DIR, 'platform-map.js'), path.join(binDir, 'platform-map.js'));

    const platformKey = `${process.platform}-${process.arch}`;
    const pkgName =
      platformKey === 'win32-x64'
        ? '@firatkutay/comrade-win32-x64'
        : `@firatkutay/comrade-${platformKey}`;
    const binaryName = process.platform === 'win32' ? 'comrade.exe' : 'comrade';

    const scopeDir = path.join(tmpRoot, 'node_modules', '@firatkutay');
    const pkgDir = path.join(scopeDir, pkgName.split('/')[1]);
    fs.mkdirSync(path.join(pkgDir, 'bin'), { recursive: true });
    fs.writeFileSync(
      path.join(pkgDir, 'package.json'),
      JSON.stringify({ name: pkgName, version: '0.0.0-test', os: [process.platform], cpu: [process.arch] })
    );

    const fakeBinaryPath = path.join(pkgDir, 'bin', binaryName);
    fn({ dispatcherPath: path.join(binDir, 'comrade.js'), fakeBinaryPath, cwd: binDir });
  } finally {
    fs.rmSync(tmpRoot, { recursive: true, force: true });
  }
}

function writeFakeBinary(fakeBinaryPath, scriptBody) {
  fs.writeFileSync(fakeBinaryPath, `#!/usr/bin/env node\n${scriptBody}\n`);
  fs.chmodSync(fakeBinaryPath, 0o755);
}

// ===========================================================================
// 3. Exit code forwarding: the dispatcher must forward the child's exact
//    (non-zero) exit code, not swallow or normalize it.
// ===========================================================================

withFixture(({ dispatcherPath, fakeBinaryPath, cwd }) => {
  writeFakeBinary(fakeBinaryPath, 'process.exit(7);');

  const result = spawnSync(process.execPath, [dispatcherPath], { cwd });

  assert.equal(result.status, 7, 'dispatcher must forward the exact child exit code');
  assert.equal(result.signal, null, 'a plain exit(7) must not be reported as signal termination');
});

// ===========================================================================
// 4. Signal forwarding: a child killed by a signal must not surface as a
//    silent 0 (or any fabricated success/failure code) -- the dispatcher
//    process itself must die by / report that same signal.
// ===========================================================================

withFixture(({ dispatcherPath, fakeBinaryPath, cwd }) => {
  writeFakeBinary(fakeBinaryPath, "process.kill(process.pid, 'SIGTERM');");

  const result = spawnSync(process.execPath, [dispatcherPath], { cwd });

  assert.notEqual(result.status, 0, 'a signal-killed child must never be reported as a clean 0 exit');
  const terminatedBySignal = result.signal === 'SIGTERM' || result.status === 128 + os.constants.signals.SIGTERM;
  assert.ok(
    terminatedBySignal,
    `expected the dispatcher to die by / report SIGTERM, got status=${result.status} signal=${result.signal}`
  );
});

// ===========================================================================
// 5. argv forwarding: arguments containing spaces, quotes, and non-ASCII
//    text must reach the child byte-exact (no shell re-splitting/mangling).
// ===========================================================================

withFixture(({ dispatcherPath, fakeBinaryPath, cwd }) => {
  writeFakeBinary(fakeBinaryPath, 'process.stdout.write(JSON.stringify(process.argv.slice(2)));');

  const trickyArgs = ['fix', 'bu hatayı çöz', '--flag="quoted value"', "single'quote", '--emoji=🚀'];
  const result = spawnSync(process.execPath, [dispatcherPath, ...trickyArgs], { cwd, encoding: 'utf8' });

  assert.equal(result.status, 0);
  assert.deepEqual(
    JSON.parse(result.stdout),
    trickyArgs,
    'argv must reach the wrapped binary exactly as passed, with no re-splitting or mangling'
  );
});

// ===========================================================================
// 6. COMRADE_MANAGED_BY=npm env signal: the child (the real Go binary)
//    must see this variable set, in addition to the rest of the parent's
//    own environment (a custom var here stands in for "the rest of
//    process.env" -- proving the dispatcher extends a COPY of it rather
//    than replacing it wholesale). internal/update.IsNPMManaged reads
//    this to refuse `comrade upgrade` self-updates for an npm-managed
//    install.
// ===========================================================================

withFixture(({ dispatcherPath, fakeBinaryPath, cwd }) => {
  writeFakeBinary(
    fakeBinaryPath,
    'process.stdout.write(JSON.stringify({ managedBy: process.env.COMRADE_MANAGED_BY, custom: process.env.COMRADE_DISPATCHER_TEST_CUSTOM_VAR }));'
  );

  const result = spawnSync(process.execPath, [dispatcherPath], {
    cwd,
    encoding: 'utf8',
    env: { ...process.env, COMRADE_DISPATCHER_TEST_CUSTOM_VAR: 'still-here' },
  });

  assert.equal(result.status, 0);
  const seen = JSON.parse(result.stdout);
  assert.equal(seen.managedBy, 'npm', 'dispatcher must set COMRADE_MANAGED_BY=npm in the child env');
  assert.equal(
    seen.custom,
    'still-here',
    'dispatcher must extend a COPY of the parent env, not replace it -- other vars must still reach the child'
  );
});

console.log('dispatcher.test.js: all assertions passed');
