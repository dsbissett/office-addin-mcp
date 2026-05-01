# office-addin-mcp

MCP server for driving Office add-ins and Excel via WebView2 and the Chrome DevTools Protocol.

`office-addin-mcp` is a Go binary that exposes a high-level tool surface for Excel and browser add-ins running inside WebView2. It speaks the [Model Context Protocol](https://modelcontextprotocol.io) over stdio or a local TCP daemon.

> v0.1.0 — see [CHANGELOG.md](CHANGELOG.md)

## Features

- **37 Excel tools** — read/write ranges, worksheets, tables, charts, pivot tables, and arbitrary `Excel.run` scripts via embedded Office.js payloads
- **Page interaction** — screenshot, snapshot, click, fill, type, hover, navigate, evaluate, console log, network log, and more
- **Add-in lifecycle** — detect, launch, and stop add-ins; open task-pane dialogs
- **~411 raw CDP methods** — code-generated from Chrome's protocol JSON, hidden by default (`--expose-raw-cdp` to enable)
- **Daemon mode** — persistent local server with automatic session reconnect for low-latency repeated calls
- **Stdio mode** — pipe-friendly MCP transport for agents that read/write JSON on stdin/stdout

## Requirements

| Requirement | Notes |
|---|---|
| **Excel + Windows 10/11** | Required for `excel.*` and `addin.*` tools |
| **Node.js 14+** | For `npx` install |
| **Go 1.22+** | Build from source only |
| macOS / Linux | Supported for `page.*` / `cdp.*` tools against headless Chrome |

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

## Excel Setup

Launch Excel with the WebView2 remote debugging port open **once per Excel session**:

**PowerShell:**

```powershell
$env:WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS = "--remote-debugging-port=9222"
Start-Process excel.exe my-workbook.xlsx
```

**Command Prompt:**

```cmd
set WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222
excel.exe my-workbook.xlsx
```

The server connects to `http://127.0.0.1:9222` by default. Pass `--browser-url` to change the address.

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

### Quick start

```bash
# List all available tools
office-addin-mcp list-tools

# Read a worksheet range
office-addin-mcp call \
  --tool excel.readRange \
  --param '{"address":"A1:D10","urlPattern":"taskpane"}' \
  --browser-url http://127.0.0.1:9222

# Capture a screenshot
office-addin-mcp call \
  --tool page.screenshot \
  --param '{"urlPattern":"taskpane","outputPath":"./shot.png"}' \
  --browser-url http://127.0.0.1:9222

# List WebView2 targets
office-addin-mcp call --tool pages.list --browser-url http://127.0.0.1:9222
```

The `urlPattern` parameter selects the WebView2 page by substring match against the target URL.

### Daemon mode

Run a persistent daemon for low-latency repeated calls. Sessions reconnect automatically within a 3-reconnect-per-60s budget.

```bash
# Terminal 1 — start the daemon
office-addin-mcp daemon --idle-timeout 30m

# Terminal 2 — calls automatically route to the daemon
office-addin-mcp call \
  --tool excel.readRange \
  --param '{"address":"A1"}' \
  --browser-url http://127.0.0.1:9222
```

The daemon writes `{port, token, pid}` to `%LOCALAPPDATA%\office-addin-mcp\daemon.json` (Windows) or `$XDG_CACHE_HOME/office-addin-mcp/daemon.json` (mode 0600). Pass `--no-daemon` to force in-process dispatch.

Watch `diagnostics.cdpRoundTrips` in the response — the first call costs 3 round-trips; subsequent calls in the same session drop to 1.

### Stdio mode (MCP protocol)

```bash
office-addin-mcp serve --stdio
```

Reads newline-delimited MCP JSON requests from stdin and writes responses to stdout. This is the transport used by all MCP clients above.

## Tool Groups

| Prefix | Count | Description |
|---|---|---|
| `excel.*` | 37 | Read/write ranges, worksheets, tables, charts, pivot tables, custom XML, `Excel.run` scripts |
| `page.*` | ~15 | Screenshot, snapshot, click, fill, type, hover, navigate, evaluate, wait, console log, network log |
| `pages.*` | 4 | List, select, close, dialog |
| `addin.*` | 6 | Detect, launch, stop, context info, CF runtime info, dialog |
| `cdp.*` | ~411 | Raw Chrome DevTools Protocol methods (hidden by default, enable with `--expose-raw-cdp`) |

## Flags & Environment Variables

| Flag | Env | Default | Description |
|---|---|---|---|
| `--browser-url` | `OAMCP_BROWSER_URL` | `http://127.0.0.1:9222` | WebView2 / Chrome debug endpoint |
| `--expose-raw-cdp` | `OFFICE_ADDIN_MCP_EXPOSE_RAW_CDP` | off | Register ~411 raw `cdp.*` methods |
| `--allow-dangerous-cdp` | `OAMCP_ALLOW_DANGEROUS_CDP` | off | Enable crash/terminate CDP methods |
| `--no-daemon` | — | — | Force in-process dispatch, skip daemon lookup |
| `--idle-timeout` | — | `30m` | Daemon: shut down after this idle period |

## Subcommands

| Command | Description |
|---|---|
| `call` | Invoke one tool. Auto-routes to a running daemon. |
| `list-tools` | Print tool catalog with name, description, and JSON Schema. |
| `daemon` | Run the persistent local TCP server. |
| `serve --stdio` | Read MCP JSON requests from stdin, write responses to stdout. |
| `version` | Print binary version. |
| `help` | Print usage. |

## License

MIT
