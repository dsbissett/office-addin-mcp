#!/usr/bin/env node
"use strict";
const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

const key = `${process.platform}-${process.arch}`;
const ext = process.platform === "win32" ? ".exe" : "";
const binPath = path.join(__dirname, `office-addin-mcp-${key}${ext}`);

// Ensure executable bit is set (npm does not always preserve it on Unix).
if (process.platform !== "win32") {
  try { fs.chmodSync(binPath, 0o755); } catch {}
}

const result = spawnSync(binPath, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  const msg = result.error.code === "ENOENT"
    ? `unsupported platform ${key}`
    : result.error.message;
  process.stderr.write(`office-addin-mcp: ${msg}\n`);
  process.exit(1);
}
process.exit(result.status ?? 1);
