'use strict';

// Maps a Node `process.platform`/`process.arch` pair to the scoped npm
// package that ships the prebuilt `comrade` binary for that target.
//
// This mirrors the build matrix in .goreleaser.yaml exactly: linux and
// darwin each ship amd64+arm64, windows ships amd64 only (windows/arm64 is
// explicitly excluded there too). Do not add an entry here without adding
// the matching build target in .goreleaser.yaml (and vice versa) — see
// scripts/build-npm-packages.sh, which assembles one platform package per
// entry below from goreleaser's dist/ output.
const PLATFORM_PACKAGES = Object.freeze({
  'linux-x64': Object.freeze({ pkg: '@firatkutay/comrade-linux-x64', binary: 'comrade' }),
  'linux-arm64': Object.freeze({ pkg: '@firatkutay/comrade-linux-arm64', binary: 'comrade' }),
  'darwin-x64': Object.freeze({ pkg: '@firatkutay/comrade-darwin-x64', binary: 'comrade' }),
  'darwin-arm64': Object.freeze({ pkg: '@firatkutay/comrade-darwin-arm64', binary: 'comrade' }),
  'win32-x64': Object.freeze({ pkg: '@firatkutay/comrade-win32-x64', binary: 'comrade.exe' }),
});

/**
 * @param {string} platform - `process.platform` value, e.g. "linux".
 * @param {string} arch - `process.arch` value, e.g. "x64".
 * @returns {string} the "platform-arch" lookup key, e.g. "linux-x64".
 */
function platformKey(platform, arch) {
  return `${platform}-${arch}`;
}

/**
 * @param {string} platform - `process.platform` value, e.g. "linux".
 * @param {string} arch - `process.arch` value, e.g. "x64".
 * @returns {{pkg: string, binary: string}|null} the scoped package name and
 *   binary filename for a supported platform/arch pair, or `null` when
 *   cli-comrade's npm distribution does not cover that combination.
 */
function lookupPlatformPackage(platform, arch) {
  return PLATFORM_PACKAGES[platformKey(platform, arch)] || null;
}

/** @returns {string[]} sorted list of every "os-arch" key the npm distribution supports. */
function supportedPlatformList() {
  return Object.keys(PLATFORM_PACKAGES).sort();
}

module.exports = { lookupPlatformPackage, supportedPlatformList, platformKey };
