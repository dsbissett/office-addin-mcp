# office-addin-mcp

> Drive Excel, Word, Outlook, PowerPoint, and OneNote add-ins from any MCP client — through one Go binary that speaks the Model Context Protocol over stdio.

`office-addin-mcp` attaches to the WebView2 runtime that Office uses to host Office.js add-ins, then exposes a high-level tool surface to LLM clients. Instead of teaching the model how to chain 15 raw Office.js primitives, the server ships one tool call per workflow.

See the [latest release](https://github.com/dsbissett/office-addin-mcp/releases/latest) · [CHANGELOG.md](CHANGELOG.md) · [PLAN-workflow-surface.md](PLAN-workflow-surface.md).

## Highlights

- **Workflow tools, not primitives.** `excel.tabulateRegion`, `word.applyEdits`, `outlook.draftReply`, `powerpoint.rebuildSlideFromOutline`, `onenote.appendToPage`, and `excel.summarizeWorkbook` each compress what used to be 5–20 calls into one Office.js batch (one CDP round-trip).
- **Server-side query DSL.** `excel.query` / `outlook.query` / `powerpoint.query` / `onenote.query` run filter / project / groupBy / agg / limit *inside* the host so a 100k-row workbook returns a 5-row answer.
- **Persistent document context cache.** `*.discover` tools snapshot workbook / document / mailbox / deck / notebook structure to `%LOCALAPPDATA%\office-addin-mcp\doccache.json` so repeat discovery calls cost zero CDP round-trips.
- **MCP Resources with live subscriptions.** Reference Office state by URI — `office://excel/Book1/Sheet1!A1:D20`, `office://word/<doc>/bookmark/<name>`, `office://outlook/inbox`, `office://pp/<deck>/slide<N>`, `office://onenote/<notebook>/<section>/<page>` — and subscribe to `notifications/resources/updated` when contents change.
- **Macro record & replay.** `macro.record_start` → run any sequence of tools → `macro.record_stop` writes a JSON macro to disk. On next launch the server registers each macro as a callable `macro.<name>` tool.
- **Auto-diagnostics on Office.js errors.** `ItemNotFound`, address parse failures, slide-index errors, and compose-vs-read mismatches are enriched in the failure envelope with `available_sheets`, `nearest_name_suggestions`, `parsed_address`, `slide_count`, `item_mode`, and a `recoveryHint` — so the model can self-correct without a second round-trip.
- **Self-healing CDP connection.** If Excel closes unexpectedly, the server detects the dead connection on the next tool call, stops the stale sideload registration, relaunches Excel (including the dev server if needed), resets the session pool, and retries the original operation — no agent intervention required. Only activates when the server owns the launch (via `--launch-addin` or a prior `addin.launch` call); attaching to an external `--browser-url` endpoint is never disturbed.
- **Progress notifications on long operations.** `addin.launch`, `addin.ensureRunning`, and macro replay stream MCP `$/progress` notifications at each phase boundary (dev-server start, sideload, CDP-ready wait, step execution) so clients can show a live status bar instead of a silent spinner.
- **Cross-host orchestration.** `office.embed` reads a range from Excel and inserts it onto a PowerPoint slide in one call.
- **Page automation fallback.** `page.*` / `pages.*` / `inspect.*` / `interact.*` cover screenshot, snapshot, click, fill, type, hover, navigate, evaluate, wait, console log, network log — same primitives against headless Chrome on macOS/Linux.
- **Per-host `runScript` escape hatch.** `excel.runScript`, `word.runScript`, `outlook.runScript`, `powerpoint.runScript`, `onenote.runScript` run arbitrary `<Host>.run` callbacks when the workflow surface isn't enough.

## Requirements

| Requirement | Notes |
|---|---|
| **Office on Windows 10/11** | Required for all `*.runScript` / `*.query` / `*.discover` / `*.applyDiff` / `addin.*` tools — Office only uses WebView2 on Windows |
| **Node.js 14+** | For the `npx` install path |
| **Go 1.22+** | Only needed to build from source |
| macOS / Linux | Supported for `page.*` / `pages.*` against headless Chrome (no Office) |

## Install

### npm (recommended)

```bash
npm install -g @dsbissett/office-addin-mcp
```

Or run on demand without installing:

```bash
npx @dsbissett/office-addin-mcp@latest --help
```

Pre-built binaries for Windows x64, macOS (Intel + Apple Silicon), and Linux (x64 + ARM64) ship via optional npm dependencies — the correct binary is fetched automatically.

### Build from source

```bash
go install github.com/dsbissett/office-addin-mcp/cmd/office-addin-mcp@latest
```

## Use it in Claude Code

One command registers the server globally:

```bash
claude mcp add office-addin-mcp -- npx -y @dsbissett/office-addin-mcp@latest
```

Or add it manually. Project scope (`.claude/mcp.json` in the repo) or user scope (`~/.claude.json` → `mcpServers`) both work:

```json
{
  "mcpServers": {
    "office-addin-mcp": {
      "command": "npx",
      "args": ["-y", "@dsbissett/office-addin-mcp@latest"]
    }
  }
}
```

Then, in any Claude Code session:

1. Open the Office host (see [Office host setup](#office-host-setup) below) so the WebView2 debug port is listening.
2. Ask Claude to do something — e.g. *"Summarize the workbook, then pivot the Sales sheet by region."* It will pick `excel.summarizeWorkbook` and `excel.query` automatically.
3. Run `/mcp` in Claude Code to inspect the tool list, or `claude mcp list` from a shell.

> **Tip:** pass `--launch-addin` and Claude can sideload the add-in project under the current working directory automatically. No manual env var needed.

## Use it in other MCP clients

<details>
<summary><strong>VS Code (GitHub Copilot)</strong></summary>

Workspace config at `.vscode/mcp.json`:

```json
{
  "servers": {
    "office-addin-mcp": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@dsbissett/office-addin-mcp@latest"]
    }
  }
}
```

Or: Command Palette → **MCP: Add Server** → paste the command.
</details>

<details>
<summary><strong>Cursor</strong></summary>

`~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (project):

```json
{
  "mcpServers": {
    "office-addin-mcp": {
      "command": "npx",
      "args": ["-y", "@dsbissett/office-addin-mcp@latest"]
    }
  }
}
```
</details>

<details>
<summary><strong>Codex (OpenAI)</strong></summary>

`~/.codex/config.toml`:

```toml
[mcp_servers.office-addin-mcp]
command = "npx"
args = ["-y", "@dsbissett/office-addin-mcp@latest"]
```
</details>

<details>
<summary><strong>Windsurf</strong></summary>

`~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "office-addin-mcp": {
      "command": "npx",
      "args": ["-y", "@dsbissett/office-addin-mcp@latest"]
    }
  }
}
```
</details>

<details>
<summary><strong>Any MCP-compatible client</strong></summary>

- **command:** `npx`
- **args:** `["-y", "@dsbissett/office-addin-mcp@latest"]`
- **transport:** `stdio`
</details>

## Office host setup

Office hosts only expose their WebView2 runtime to CDP when started with a debug-port env var. The variable is shared across every Office app, so the same setup works for all five:

**PowerShell**

```powershell
$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = "--remote-debugging-port=9222"
Start-Process excel.exe my-workbook.xlsx
# or:  Start-Process winword.exe my-document.docx
# or:  Start-Process outlook.exe
# or:  Start-Process powerpnt.exe my-deck.pptx
# or:  Start-Process onenote.exe
```

**Command Prompt**

```cmd
set WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222
excel.exe my-workbook.xlsx
```

The server probes `http://127.0.0.1:9222` by default. Override with `--browser-url` or `--ws-endpoint`. Or skip the setup entirely with `--launch-addin` — the server will detect the Office add-in project under cwd and sideload it through `office-addin-debugging`.

## Tool surface

Phase 0 of [PLAN-workflow-surface.md](PLAN-workflow-surface.md) deleted the raw `cdp.*` surface; everything below is the workflow-shaped replacement. Each workflow tool is one MCP call → one Office.js batch → one CDP round-trip.

### Excel
| Tool | Purpose |
|---|---|
| `excel.summarizeWorkbook` | Sheets, tables, named ranges, used-range bounds — single call |
| `excel.tabulateRegion` | Load a range, return rows-as-objects + per-column type tags |
| `excel.query` | Server-side filter / project / groupBy / agg / limit DSL |
| `excel.discover` | Persistent workbook fingerprint snapshot (doccache) |
| `excel.applyDiff` | Batch cell/range patches in one `Excel.run` |
| `excel.runScript` | Run an arbitrary `Excel.run` callback |

### Word
| Tool | Purpose |
|---|---|
| `word.discover` | Document structure snapshot |
| `word.applyEdits` | Batch find/replace edits in one `Word.run` |
| `word.runScript` | Run an arbitrary `Word.run` callback |

### Outlook
| Tool | Purpose |
|---|---|
| `outlook.query` | Query the mailbox / current item |
| `outlook.discover` | Mailbox + active-item snapshot |
| `outlook.draftReply` | Set subject and/or body on a compose-mode item |
| `outlook.runScript` | Run an arbitrary callback against `Office.context.mailbox` |

### PowerPoint
| Tool | Purpose |
|---|---|
| `powerpoint.query` | Slide-level query DSL |
| `powerpoint.discover` | Deck structure snapshot |
| `powerpoint.rebuildSlideFromOutline` | Rewrite a slide's title and/or body bullets |
| `powerpoint.runScript` | Run an arbitrary `PowerPoint.run` callback |

### OneNote
| Tool | Purpose |
|---|---|
| `onenote.query` | Notebook / section / page query DSL |
| `onenote.discover` | Notebook structure snapshot |
| `onenote.appendToPage` | Append HTML and/or bullets to a page |
| `onenote.runScript` | Run an arbitrary `OneNote.run` callback |

### Cross-host & macros
| Tool | Purpose |
|---|---|
| `office.embed` | Read an Excel range and insert it on a PowerPoint slide |
| `macro.record_start` | Begin capturing subsequent tool calls into a named macro |
| `macro.record_stop` | Finalize and persist the macro to disk |
| `macro.<name>` | Replay a persisted macro (auto-registered on next startup) |

### Add-in lifecycle & generic page automation
| Tool group | Purpose |
|---|---|
| `addin.*` | `ensureRunning` (**start here** — probes then launches if needed), `launch`, `stop`, `detect`, `status`, `contextInfo`, `cfRuntimeInfo`, `openDialog`, `dialogSubscribe`, `dialogClose`, `listTargets` |
| `page.*` | `screenshot`, `snapshot`, `click`, `fill`, `typeText`, `hover`, `navigate`, `evaluate`, `pressKey`, `waitFor`, `consoleLog`, `networkLog`, `networkBody` |
| `pages.*` | `list`, `select`, `close`, `handleDialog` |
| `inspect.*` / `interact.*` | DOM / accessibility inspection and higher-level interaction primitives |

## MCP Resources

LLM clients can reference Office state by URI instead of re-fetching it every turn. Five resource templates are registered on startup:

| URI template | Backed by |
|---|---|
| `office://excel/<workbook>/<sheet>!<range>` | `excel.tabulateRegion` |
| `office://word/<doc>/bookmark/<name>` | `word.runScript` (inline script) |
| `office://outlook/<folder>` | `outlook.query` |
| `office://pp/<deck>/slide<N>` | `powerpoint.query` |
| `office://onenote/<notebook>/<section>/<page>` | `onenote.query` |

Resources support `resources/subscribe`. The server polls fingerprints every 30s and emits `notifications/resources/updated` when content changes; `resources/unsubscribe` (or client disconnect) tears down the polling goroutine.

## Performance notes

- `urlPattern` selects the WebView2 page by substring match against the target URL. After the first call against a session, the selector cache drops `diagnostics.cdpRoundTrips` from ~3 to 1 on subsequent calls.
- The session pool allows 3 reconnects per 60-second window. Exhaustion surfaces as `connection/session_acquire_failed`. If the server owns the launch, it attempts one automatic stop+relaunch cycle before surfacing that error, so transient Excel restarts are transparent to the agent.
- DocCache snapshots are atomic-rename writes, mode 0600. Disable with `--no-doccache`.

## Flags & environment variables

| Flag | Env | Default | Description |
|---|---|---|---|
| `--browser-url` | — | `http://127.0.0.1:9222` | WebView2 / Chrome debug endpoint |
| `--ws-endpoint` | — | — | Direct browser WebSocket URL (overrides `--browser-url`) |
| `--log-file` | — | stderr | Append diagnostics to a file instead of stderr |
| `--log-level` | — | `info` | slog level: `debug`, `info`, `warn`, `error` |
| `--launch-addin` | — | off | Auto-detect and sideload the Office add-in project under cwd at startup, only if no CDP endpoint is reachable |
| `--launch-excel` | — | off | Deprecated alias for `--launch-addin` |
| `--no-doccache` | — | off | Disable the persistent document discovery cache (`*.discover` still runs; reads/writes to `doccache.json` are skipped) |
| `--allow-dangerous-cdp` | `OAMCP_ALLOW_DANGEROUS_CDP` | off | Enable crash/terminate CDP methods |
| `--version` | — | — | Print binary version and exit |

The binary takes no positional subcommands — it speaks MCP over stdio. The legacy `call` / `daemon` / `serve --stdio` subcommands have been removed.

## Layout

```
cmd/office-addin-mcp/   main entry
internal/tools/         registry, dispatcher, envelope, diagnostics
internal/cdp/           hand-rolled CDP WebSocket client
internal/webview2/      endpoint discovery
internal/session/       session pool + reconnect budget
internal/officejs/      Office.js executor + payloads
internal/js/            embedded Office.js scripts
internal/doccache/      persistent document discovery cache
internal/resources/     office:// URI parsing + provider + polling watcher
internal/recorder/      macro recorder store
internal/tools/macrotool/  macro.record_start / record_stop / replay
internal/mcp/           SDK server wiring + resource registration
```

## License

MIT
