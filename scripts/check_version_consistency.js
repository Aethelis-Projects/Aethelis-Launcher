#!/usr/bin/env node
// check_version_consistency.js — single-source version gate.
//
// VERSION at the repo root is the release identity. The ldflags injection
// (-X main.version) uses it in CI; wails.json / package.json metadata must
// not drift (they had: 0.7.1 vs 0.6.1 before v0.7.2). Also enforces that the
// CHANGELOG carries a section for the version being shipped.
const fs = require("fs");

function read(p) {
  try {
    return fs.readFileSync(p, "utf8");
  } catch (_e) {
    console.error("MISSING: " + p);
    process.exit(1);
  }
}

const version = read("VERSION").trim();
if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) {
  console.error(`VIOLATION: VERSION file must be a plain semver (found "${version}")`);
  process.exit(1);
}

const pkg = JSON.parse(read("frontend/package.json"));
const wails = JSON.parse(read("wails.json"));
const changelog = read("CHANGELOG.md");

const checks = [
  ["frontend/package.json version", pkg.version, version],
  ["wails.json version", wails.version, version],
  ["wails.json info.productVersion", wails.info && wails.info.productVersion, version],
];
if (process.argv[2] && process.argv[2].startsWith("v")) {
  checks.push(["release tag", process.argv[2].slice(1), version]);
}

let failed = 0;
for (const [what, got, want] of checks) {
  if (got !== want) {
    console.error(`VIOLATION: ${what} = "${got}", expected "${want}" (VERSION is the single source).`);
    failed++;
  }
}
if (!changelog.includes(`## [${version}]`)) {
  console.error(`VIOLATION: CHANGELOG.md has no "## [${version}]" section.`);
  failed++;
}
if (failed) {
  process.exit(1);
}
console.log(`PASS: version ${version} consistent across VERSION / package.json / wails.json / CHANGELOG.`);
