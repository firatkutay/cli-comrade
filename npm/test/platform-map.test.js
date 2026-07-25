'use strict';

// Plain assertion-script test (no test-framework dependency) -- run via
// `node platform-map.test.js`; throws (non-zero exit) on any failed
// assertion. Aggregated by npm/test/run-node-tests.sh.

const assert = require('assert/strict');
const { lookupPlatformPackage, supportedPlatformList, platformKey } = require('../main/bin/platform-map');

// --- lookupPlatformPackage: every supported platform resolves exactly ----

assert.deepEqual(
  lookupPlatformPackage('linux', 'x64'),
  { pkg: '@firatkutay/comrade-linux-x64', binary: 'comrade' },
  'linux-x64 must resolve to the linux-x64 platform package with the unix binary name'
);

assert.deepEqual(
  lookupPlatformPackage('linux', 'arm64'),
  { pkg: '@firatkutay/comrade-linux-arm64', binary: 'comrade' },
  'linux-arm64 must resolve to the linux-arm64 platform package'
);

assert.deepEqual(
  lookupPlatformPackage('darwin', 'x64'),
  { pkg: '@firatkutay/comrade-darwin-x64', binary: 'comrade' },
  'darwin-x64 must resolve to the darwin-x64 platform package'
);

assert.deepEqual(
  lookupPlatformPackage('darwin', 'arm64'),
  { pkg: '@firatkutay/comrade-darwin-arm64', binary: 'comrade' },
  'darwin-arm64 must resolve to the darwin-arm64 platform package'
);

assert.deepEqual(
  lookupPlatformPackage('win32', 'x64'),
  { pkg: '@firatkutay/comrade-win32-x64', binary: 'comrade.exe' },
  'win32-x64 must resolve to the win32-x64 platform package with the .exe binary name'
);

// --- lookupPlatformPackage: unsupported combinations return null ---------

assert.equal(
  lookupPlatformPackage('win32', 'arm64'),
  null,
  'win32-arm64 is explicitly excluded from .goreleaser.yaml\'s build matrix and must not resolve'
);

assert.equal(lookupPlatformPackage('sunos', 'x64'), null, 'an entirely unsupported OS must not resolve');
assert.equal(lookupPlatformPackage('linux', 'ia32'), null, 'an entirely unsupported arch must not resolve');
assert.equal(lookupPlatformPackage('', ''), null, 'empty platform/arch must not resolve');

// --- supportedPlatformList: exact, sorted set -----------------------------

assert.deepEqual(
  supportedPlatformList(),
  ['darwin-arm64', 'darwin-x64', 'linux-arm64', 'linux-x64', 'win32-x64'],
  'supportedPlatformList must return exactly the 5-platform matrix, sorted'
);

// --- platformKey: pure string join ----------------------------------------

assert.equal(platformKey('linux', 'x64'), 'linux-x64');
assert.equal(platformKey('win32', 'arm64'), 'win32-arm64');

console.log('platform-map.test.js: all assertions passed');
