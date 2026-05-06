# office-addin-mcp

MCP server for driving Office add-ins (Excel, Word, Outlook, PowerPoint, OneNote) via WebView2 and the Chrome DevTools Protocol.

`office-addin-mcp` is a Go binary that exposes a high-level tool surface for Office add-ins running inside WebView2. It speaks the [Model Context Protocol](https://modelcontextprotocol.io) over stdio.

See the [latest release](https://github.com/dsbissett/office-addin-mcp/releases/latest) and [CHANGELOG.md](CHANGELOG.md).

- **Per-host `runScript` escape hatch across 5 apps** — `excel.runScript`, `word.runScript`, `outlook.runScript`, `powerpoint.runScript`, `onenote.runScript`. Run arbitrary `<Host>.run` Office.js code against the active document.
- **Phase A workflow tools** — `excel.tabulateRegion`, `excel.applyDiff`, `excel.summarizeWorkbook`, `word.applyEdits`, `outlook.draftReply`, `powerpoint.rebuildSlideFromOutline`, `onenote.appendToPage`. Each one is a single tool call that replaces what used to be 5–20 primitive calls — see [PLAN-workflow-surface.md](PLAN-workflow-surface.md) for the design rationale.
- **Cross-host orchestration** — `office.embed` reads from one Office host (Excel) and writes to another (PowerPoint) in a single tool call.
- **Page interaction** — screenshot, snapshot, click, fill, type, hover, navigate, evaluate, console log, network log, and more
- **Add-in lifecycle** — detect, launch, and stop add-ins for any Office host; open task-pane dialogs
- **MCP-native stdio transport** — plug into Claude Code, Cursor, VS Code GitHub Copilot, Codex, Windsurf, or any MCP-compatible client

## Requirements

| Requirement | Notes |
|---|---|
| **Office on Windows 10/11** | Required for `excel.*` / `word.*` / `outlook.*` / `powerpoint.*` / `onenote.*` / `addin.*` tools (Office uses WebView2 only on Windows) |
| **Node.js 14+** | For `npx` install |
| **Go 1.22+** | Build from source only |
| macOS / Linux | Supported for `page.*` tools against headless Chrome |

## Install

### npm (recommended)

```bash
npm install -g @dsbissett/office-addin-mcp
```

Or run without installing:

```bash
npx @dsbissett/office-addin-mcp@latest --help
```

Pre-built binaries for Windows x64, macOS (Intel + Apple Silicon), and Linux (x64 + ARM64) are installed automatically via optional dependencies.

### Build from source

```bash
go install github.com/dsbissett/office-addin-mcp/cmd/office-addin-mcp@latest
```

## Office Host Setup

Launch the Office host (Excel, Word, Outlook, PowerPoint, or OneNote) with the WebView2 remote debugging port open **once per host session**. The env var is shared by every Office WebView2 instance, so the same setup works for all five apps:

**PowerShell:**

```powershell
$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = "--remote-debugging-port=9222"
Start-Process excel.exe my-workbook.xlsx
# or:    Start-Process winword.exe my-document.docx
# or:    Start-Process outlook.exe
# or:    Start-Process powerpnt.exe my-deck.pptx
# or:    Start-Process onenote.exe
```

**Command Prompt:**

```cmd
set WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222
excel.exe my-workbook.xlsx
```

The server connects to `http://127.0.0.1:9222` by default. Pass `--browser-url` to change the address. Or pass `--launch-addin` and the server will detect the project under your cwd and sideload it via `office-addin-debugging` automatically — no manual env var needed.

## MCP Client Configuration

### Claude Code

```bash
claude mcp add office-addin-mcp -- npx -y @dsbissett/office-addin-mcp@latest
```

Or add manually to `.claude/mcp.json` (project) or `~/.claude/mcp.json` (global):

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

### VS Code (GitHub Copilot)

Add to your workspace `.vscode/mcp.json`:

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

Alternatively, open the Command Palette → **MCP: Add Server** and paste the command.

### Cursor

Add to `~/.cursor/mcp.json` (global) or `.cursor/mcp.json` (project):

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

### Codex (OpenAI)

Add to `~/.codex/config.toml`:

```toml
[mcp_servers.office-addin-mcp]
command = "npx"
args = ["-y", "@dsbissett/office-addin-mcp@latest"]
```

### Windsurf

Add to `~/.codeium/windsurf/mcp_config.json`:

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

### Generic (any MCP-compatible client)

The server speaks MCP over stdio. Configure your client with:

- **command:** `npx`
- **args:** `["-y", "@dsbissett/office-addin-mcp@latest"]`
- **transport:** `stdio`

## Usage

The binary speaks MCP over stdio. Tools are listed and invoked by your MCP client (Claude Code, Cursor, VS Code Copilot, etc.) — see [MCP Client Configuration](#mcp-client-configuration) above for setup.

The `urlPattern` parameter accepted by most tools selects the WebView2 page by substring match against the target URL. After the first call against a session, the selector cache drops `diagnostics.cdpRoundTrips` from ~3 to 1 on subsequent calls.

## Tool Groups

Phase 0 of [PLAN-workflow-surface.md](PLAN-workflow-surface.md) deleted the raw `cdp.*` surface and the host primitive tools; Phase A reintroduces a small, workflow-shaped surface. Each workflow tool is one MCP call that runs a single Office.js batch (one CDP round-trip), replacing the multi-call primitive sequences agents previously had to compose.

| Tool | Description |
|---|---|
| `excel.runScript` | Run an `Excel.run` callback against the active workbook |
| `excel.tabulateRegion` | Load a range and return rows-as-objects + per-column type tags |
| `excel.applyDiff` | Apply a batch of cell/range patches in one `Excel.run` |
| `excel.summarizeWorkbook` | One-call workbook discovery: sheets, tables, named ranges, used-range bounds |
| `word.runScript` | Run a `Word.run` callback against the active document |
| `word.applyEdits` | Apply a batch of find/replace edits in one `Word.run` |
| `outlook.runScript` | Run a custom callback against `Office.context.mailbox` |
| `outlook.draftReply` | Set subject and/or body on a compose-mode item in one call |
| `powerpoint.runScript` | Run a `PowerPoint.run` callback against the active presentation |
| `powerpoint.rebuildSlideFromOutline` | Rewrite a slide's title and/or body bullets in one `PowerPoint.run` |
| `onenote.runScript` | Run a `OneNote.run` callback against the active notebook |
| `onenote.appendToPage` | Append HTML and/or bullets to a OneNote page in one call |
| `office.embed` | Cross-host: read an Excel range and insert it onto a PowerPoint slide |
| `page.*` | Screenshot, snapshot, click, fill, type, hover, navigate, evaluate, wait, console log, network log |
| `pages.*` | List, select, close, dialog |
| `addin.*` | Detect, launch, stop, context info, CF runtime info, dialog |
| `inspect.*` / `interact.*` | DOM / accessibility inspection and high-level interaction primitives |

## Flags & Environment Variables

| Flag | Env | Default | Description |
|---|---|---|---|
| `--browser-url` | — | `http://127.0.0.1:9222` | WebView2 / Chrome debug endpoint |
| `--ws-endpoint` | — | — | Direct browser WebSocket URL (overrides `--browser-url`) |
| `--log-file` | — | stderr | Append diagnostics to a file instead of stderr |
| `--log-level` | — | `info` | slog level: `debug`, `info`, `warn`, `error` |
| `--launch-addin` | — | off | Auto-detect and launch the Office add-in under cwd at startup if no CDP endpoint is reachable. Works for any host (Excel, Word, Outlook, PowerPoint, OneNote) |
| `--launch-excel` | — | off | Deprecated alias for `--launch-addin` |
| `--allow-dangerous-cdp` | `OAMCP_ALLOW_DANGEROUS_CDP` | off | Enable crash/terminate CDP methods |
| `--version` | — | — | Print binary version and exit |

The binary takes no positional subcommands — it speaks MCP over stdio. Earlier `call` / `daemon` / `serve --stdio` subcommands have been removed.

## License

MIT
