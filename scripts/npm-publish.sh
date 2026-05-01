#!/usr/bin/env bash
# npm-publish.sh — build binaries and publish all npm packages.
#
# Usage:
#   ./scripts/npm-publish.sh              # reads version from current git tag
#   ./scripts/npm-publish.sh 0.2.0        # explicit version
#   ./scripts/npm-publish.sh --dry-run    # build + stage binaries, skip npm publish
#
# Prerequisites: goreleaser, npm (logged in as @dsbissett), git tag on HEAD.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

DRY_RUN=false
VERSION=""
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    *)         VERSION="$arg" ;;
  esac
done

if [[ -z "$VERSION" ]]; then
  VERSION="$(git describe --tags --exact-match 2>/dev/null | sed 's/^v//')" || {
    echo "error: no git tag on HEAD and no version argument given" >&2
    echo "  tag the commit: git tag v0.x.y" >&2
    echo "  or pass version: $0 0.x.y" >&2
    exit 1
  }
fi

echo "==> version: $VERSION"

# Update version in all package.json files.
update_version() {
  local file="$1"
  node -e "
    const fs = require('fs');
    const p = JSON.parse(fs.readFileSync('$file'));
    p.version = '$VERSION';
    // update optionalDependencies versions too
    if (p.optionalDependencies) {
      for (const k of Object.keys(p.optionalDependencies)) {
        p.optionalDependencies[k] = '$VERSION';
      }
    }
    fs.writeFileSync('$file', JSON.stringify(p, null, 2) + '\n');
  "
}

echo "==> updating package.json versions to $VERSION"
update_version npm/main/package.json
for platform in win32-x64 darwin-x64 darwin-arm64 linux-x64 linux-arm64; do
  update_version "npm/$platform/package.json"
done
update_version mcp.json
update_version .claude-plugin/plugin.json
update_version .claude-plugin/marketplace.json

# Build all platform binaries.
echo "==> building binaries (goreleaser build --clean)"
goreleaser build --clean

# Copy each binary into its npm package directory.
copy_bin() {
  local platform="$1" glob="$2"
  local src
  src="$(ls $glob)"
  echo "  $platform <- $src"
  cp "$src" "npm/$platform/"
}

echo "==> staging binaries"
copy_bin win32-x64    "dist/office-addin-mcp_windows_amd64*/office-addin-mcp.exe"
copy_bin darwin-x64   "dist/office-addin-mcp_darwin_amd64*/office-addin-mcp"
copy_bin darwin-arm64 "dist/office-addin-mcp_darwin_arm64*/office-addin-mcp"
copy_bin linux-x64    "dist/office-addin-mcp_linux_amd64*/office-addin-mcp"
copy_bin linux-arm64  "dist/office-addin-mcp_linux_arm64*/office-addin-mcp"

if $DRY_RUN; then
  echo "==> --dry-run: skipping npm publish"
  echo "    staged binaries:"
  for platform in win32-x64 darwin-x64 darwin-arm64 linux-x64 linux-arm64; do
    ls -lh "npm/$platform/office-addin-mcp"* 2>/dev/null || true
  done
  exit 0
fi

# Publish platform packages first, then the main wrapper.
echo "==> publishing platform packages"
for platform in win32-x64 darwin-x64 darwin-arm64 linux-x64 linux-arm64; do
  echo "  @dsbissett/office-addin-mcp-$platform@$VERSION"
  (cd "npm/$platform" && npm publish --access public)
done

echo "==> publishing @dsbissett/office-addin-mcp@$VERSION"
(cd npm/main && npm publish --access public)

echo "==> done. install with: npm install -g @dsbissett/office-addin-mcp"
