#!/usr/bin/env node
'use strict';

const path = require('path');
const os = require('os');
const { spawnSync } = require('child_process');
const { lookupPlatformPackage, supportedPlatformList } = require('./platform-map');

const OTHER_INSTALL_CHANNELS = [
  '  - install script: curl -fsSL https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.sh | sh',
  '  - Homebrew (macOS/Linux): brew install firatkutay/tap/comrade',
  '  - Scoop (Windows):        scoop bucket add firatkutay https://github.com/firatkutay/scoop-bucket && scoop install comrade',
  '  - winget (Windows):       winget install cli.comrade',
  '  - .deb/.rpm packages:     https://github.com/firatkutay/cli-comrade/releases',
];

/**
 * Builds the friendly, actionable message printed when no prebuilt binary
 * could be resolved for the current platform/arch — instead of letting a
 * raw `MODULE_NOT_FOUND` stack trace reach the user.
 */
function unsupportedPlatformMessage(platform, arch, reason) {
  return [
    `cli-comrade: no prebuilt binary available for this platform ("${platform}-${arch}").`,
    '',
    reason,
    '',
    `Supported npm platforms: ${supportedPlatformList().join(', ')}`,
    '',
    'Install cli-comrade a different way instead:',
    ...OTHER_INSTALL_CHANNELS,
  ].join('\n');
}

/**
 * Resolves the absolute path to the platform-specific `comrade` binary.
 * Returns `{ binaryPath }` on success or `{ error }` (a ready-to-print
 * message) when no matching, installed platform package could be found.
 */
function resolveBinaryPath() {
  const entry = lookupPlatformPackage(process.platform, process.arch);
  if (!entry) {
    return {
      error: unsupportedPlatformMessage(
        process.platform,
        process.arch,
        'This platform/architecture combination is not part of the npm distribution matrix.'
      ),
    };
  }

  let pkgJsonPath;
  try {
    pkgJsonPath = require.resolve(`${entry.pkg}/package.json`);
  } catch (_err) {
    return {
      error: unsupportedPlatformMessage(
        process.platform,
        process.arch,
        `The optional dependency "${entry.pkg}" is not installed. This can happen when npm ` +
          'skipped optionalDependencies (--no-optional / --omit=optional), when installing ' +
          'offline without a warm cache, or on a registry mirror that does not carry it.'
      ),
    };
  }

  return { binaryPath: path.join(path.dirname(pkgJsonPath), 'bin', entry.binary) };
}

/**
 * Runs the resolved binary with the given argv, forwarding stdio, exit
 * code, and signal exactly. Returns the process exit code to use (the
 * caller decides whether to `process.exit` with it, so tests can call this
 * without terminating the test runner).
 */
function runBinary(binaryPath, argv) {
  const result = spawnSync(binaryPath, argv, { stdio: 'inherit' });

  if (result.error) {
    console.error(`cli-comrade: failed to launch "${binaryPath}": ${result.error.message}`);
    return 1;
  }

  if (result.signal) {
    // The child was terminated by a signal (Ctrl-C -> SIGINT, SIGTERM,
    // SIGKILL, ...). Re-raise the same signal against ourselves so the
    // parent shell observes the real signal-based termination instead of
    // a fabricated exit code.
    try {
      process.kill(process.pid, result.signal);
    } catch (_err) {
      // ignore - fall through to the numeric fallback below
    }
    const signalNumber = (os.constants.signals && os.constants.signals[result.signal]) || 1;
    return 128 + signalNumber;
  }

  return result.status === null ? 1 : result.status;
}

function main() {
  const resolved = resolveBinaryPath();
  if (resolved.error) {
    console.error(resolved.error);
    process.exit(1);
    return;
  }

  const exitCode = runBinary(resolved.binaryPath, process.argv.slice(2));
  process.exit(exitCode);
}

if (require.main === module) {
  main();
}

module.exports = { resolveBinaryPath, runBinary, unsupportedPlatformMessage, main };
