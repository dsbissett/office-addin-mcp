# office-addin-mcp

A Go binary that drives Office add-ins running inside WebView2 over the
Chrome DevTools Protocol. It speaks a small, stable JSON tool surface
(`{ok, data, error, diagnostics}`) designed for AI agents.

> v0.1.0 — see [CHANGELOG.md](CHANGELOG.md).

## What it does

- Exposes a high-level Office add-in tool surface for AI agents:
  `addin.*` (lifecycle / detect / launch / stop / dialog),
  `pages.*` (target enumeration & selection), `page.*` (snapshot,
  screenshot, click, fill, evaluate, navigate, …), and `excel.*` (37
  tools backed by embedded Office.js payloads). This is what
  `tools/list` advertises by default.
- Optionally exposes ~411 raw CDP methods (`cdp.<domain>.<method>`,
  code-generated from Chrome's vendored protocol JSON) when run with
  `--expose-raw-cdp`. See [docs/cdp-tools.md](docs/cdp-tools.md).
- Reads / writes Excel ranges, manages worksheets, creates tables, runs
  arbitrary `Excel.run` scripts via embedded Office.js payloads.

## Install

```bash
go install github.com/dsbissett/office-addin-mcp/cmd/office-addin-mcp@v0.1.0
```

Or grab a release archive (see `.goreleaser.yml` / `goreleaser release
--snapshot --clean` for a local build).

Requirements: Go 1.22+, Windows 10/11 with Excel + a manifest-based
add-in for the Excel-specific tools. macOS and Linux work for the CDP /
browser tools against headless Chrome.

## Quick start

1. **Launch Excel with the WebView2 debug port** (one time per Excel
   session — auto-launch is deferred per PLAN.md §11 Q7):

   ```powershell
   $env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = "--remote-debugging-port=9222"
   excel.exe my-workbook.xlsx
   ```

2. **List the targets** (the legacy `cdp.getTargets` returns a filtered list; `cdp.target.getTargets` is the raw CDP passthrough):

   ```bash
   office-addin-mcp call --tool cdp.getTargets --browser-url http://127.0.0.1:9222
   ```

   **Capture a screenshot to disk** (binary tool, optional `outputPath`):

   ```bash
   office-addin-mcp call \
     --tool cdp.page.captureScreenshot \
     --param '{"urlPattern":"taskpane","outputPath":"./shot.png"}' \
     --browser-url http://127.0.0.1:9222
   ```

3. **Read a range:**

   ```bash
   office-addin-mcp call \
     --tool excel.readRange \
     --param '{"address":"A1:D10","urlPattern":"taskpane"}' \
     --browser-url http://127.0.0.1:9222
   ```

   The `urlPattern` selector picks the WebView2 page hosting the add-in
   (substring match against the target URL).

4. **List every tool + JSON Schema:**

   ```bash
   office-addin-mcp list-tools
   ```

## Daemon mode

For repeated calls (especially against the same workbook), run a daemon.
`call` autoroutes to it via a well-known socket file.

```bash
# Start the daemon (foreground; Ctrl-C to stop).
office-addin-mcp daemon --idle-timeout 30m

# In another shell — `call` finds the daemon and routes there.
office-addin-mcp call --tool excel.readRange --param '{"address":"A1"}' \
  --browser-url http://127.0.0.1:9222
```

Watch `diagnostics.cdpRoundTrips` in the response — the first call pays
3 (`Target.getTargets` + `Target.attachToTarget` + `Runtime.evaluate`);
subsequent calls in the same session drop to 1.

The daemon writes `{port, token, pid}` to
`%LOCALAPPDATA%\office-addin-mcp\daemon.json` (Windows) or
`$XDG_CACHE_HOME/office-addin-mcp/daemon.json` (other) with mode 0600.

To force in-process dispatch, pass `--no-daemon` to `call`.

### Raw CDP surface (`--expose-raw-cdp`)

By default the server hides the raw `cdp.*` tool surface; agents see
only the high-level `addin.*`, `pages.*`, `page.*`, and `excel.*`
tools. Pass `--expose-raw-cdp` (or set
`OFFICE_ADDIN_MCP_EXPOSE_RAW_CDP=1`) to additionally register the
~411 code-generated CDP methods plus the hand-written
`cdp.selectTarget` cache primer.

### Dangerous CDP methods

A small set of generated tools (`cdp.browser.crash`,
`cdp.runtime.terminateExecution`, `cdp.network.clearBrowserCache`, …
see [docs/cdp-tools.md](docs/cdp-tools.md)) refuse by default. Pass
`--allow-dangerous-cdp` (or set `OAMCP_ALLOW_DANGEROUS_CDP=1`) to
enable them. Refusals come back as
`{ok:false, error:{category:"unsupported", code:"dangerous_disabled"}}`.

## Stdio mode

For agents that prefer pipes over TCP:

```bash
office-addin-mcp serve --stdio
# Then write newline-delimited JSON requests to stdin; envelopes come
# back on stdout. Sessions persist for the stream lifetime.
```

Request shape:

```json
{"tool":"cdp.evaluate","params":{"expression":"1+1"},"sessionId":"default","endpoint":{"browserUrl":"http://127.0.0.1:9222"}}
```

## Subcommands

| Command       | Purpose                                                       |
| ------------- | ------------------------------------------------------------- |
| `call`        | Invoke one tool. Auto-routes to a running daemon.             |
| `list-tools`  | Print tool catalog (name + description + JSON Schema).        |
| `daemon`      | Run the persistent TCP server.                                |
| `serve --stdio` | Read JSON requests on stdin, write envelopes on stdout.     |
| `version`     | Print binary version.                                         |
| `help`        | Print usage.                                                  |

## Documentation

- [docs/architecture.md](docs/architecture.md) — package layout, data
  flow, session lifecycle, code generation pipeline.
- [docs/tool-contracts.md](docs/tool-contracts.md) — envelope spec, error
  categories, tool catalog with stability marks, dangerous-method
  gating, binary `outputPath` semantics.
- [docs/cdp-tools.md](docs/cdp-tools.md) — full ~411-tool index by
  domain, regenerated by `go generate ./...`.
- [docs/migration-notes.md](docs/migration-notes.md) — TypeScript
  `excel-webview2-mcp` → Go `office-addin-mcp` parity table, manual
  Excel acceptance checklist, protocol-roll procedure.

## License

TBD.
