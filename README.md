# office-addin-mcp

A Go binary that drives Office add-ins running inside WebView2 over the
Chrome DevTools Protocol. It speaks a small, stable JSON tool surface
(`{ok, data, error, diagnostics}`) designed for AI agents.

> v0.1.0 — see [CHANGELOG.md](CHANGELOG.md).

## What it does

- Lists / selects CDP targets, evaluates JS, navigates pages.
- Reads / writes Excel ranges, manages worksheets, creates tables, runs
  arbitrary `Excel.run` scripts via embedded Office.js payloads.
- Runs as a one-shot CLI **or** a long-lived daemon. The daemon amortizes
  the CDP attach cost across calls — a 10-call sequence does one
  `Target.attachToTarget` and re-uses the cached selection for the rest.

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

2. **List the targets:**

   ```bash
   office-addin-mcp call --tool cdp.getTargets --browser-url http://127.0.0.1:9222
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
  flow, session lifecycle.
- [docs/tool-contracts.md](docs/tool-contracts.md) — envelope spec, error
  categories, tool catalog with stability marks.
- [docs/migration-notes.md](docs/migration-notes.md) — TypeScript
  `excel-webview2-mcp` → Go `office-addin-mcp` parity table and manual
  Excel acceptance checklist.

## License

TBD.
