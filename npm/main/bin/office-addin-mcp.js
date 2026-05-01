#!/usr/bin/env node
"use strict";
const { spawnSync } = require("child_process");

const PLATFORMS = {
  "win32-x64":    ["@dsbissett/office-addin-mcp-win32-x64",    "office-addin-mcp.exe"],
  "darwin-x64":   ["@dsbissett/office-addin-mcp-darwin-x64",   "office-addin-mcp"],
  "darwin-arm64": ["@dsbissett/office-addin-mcp-darwin-arm64", "office-addin-mcp"],
  "linux-x64":    ["@dsbissett/office-addin-mcp-linux-x64",    "office-addin-mcp"],
  "linux-arm64":  ["@dsbissett/office-addin-mcp-linux-arm64",  "office-addin-mcp"],
};

const key = `${process.platform}-${process.arch}`;
const entry = PLATFORMS[key];
if (!entry) {
  process.stderr.write(`office-addin-mcp: unsupported platform ${key}\n`);
  process.exit(1);
}

const [pkg, bin] = entry;
let binPath;
try {
  binPath = require.resolve(`${pkg}/${bin}`);
} catch {
  process.stderr.write(`office-addin-mcp: platform package ${pkg} is not installed\n`);
  process.exit(1);
}

const result = spawnSync(binPath, process.argv.slice(2), { stdio: "inherit" });
process.exit(result.status ?? 1);
