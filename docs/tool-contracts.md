# Tool contracts

This is the public contract for agents that drive `office-addin-mcp`.
Anything outside this document is implementation detail and may change
without a version bump. Anything inside is gated by golden tests in
`internal/tools/testdata/golden/` and by the `EnvelopeVersion` constant
in [`internal/tools/result.go`](../internal/tools/result.go).

## The envelope

Every tool call returns the same JSON shape:

```json
{
  "ok": true,
  "data": { /* tool-specific */ },
  "diagnostics": {
    "tool": "excel.readRange",
    "envelopeVersion": "v0.2",
    "sessionId": "default",
    "cdpSessionId": "C01BDFF55141C0C02B00AC268FE825A5",
    "targetId": "A9C90F5919D0EAACC7D64B5778B520A8",
    "endpoint": "http://127.0.0.1:9222",
    "cdpRoundTrips": 1,
    "durationMs": 3
  }
}
```

On failure: `ok: false`, `data` omitted, `error` populated:

```json
{
  "ok": false,
  "error": {
    "code": "ItemNotFound",
    "message": "Worksheet 'Bogus' not found.",
    "category": "office_js",
    "retryable": false,
    "details": { "debugInfo": { "errorLocation": "workbook.worksheets.getItem" } }
  },
  "diagnostics": { /* always present */ }
}
```

### Envelope versions

| Version | Stable since | Notes                                                              |
| ------- | ------------ | ------------------------------------------------------------------ |
| `v0.1`  | Phase 3      | Initial uniform envelope.                                          |
| `v0.2`  | Phase 5 / v0.1.0 | `sessionId` is the user/Phase-5 session; `cdpSessionId` carries the CDP flatten session; `cdpRoundTrips` added. |

Any change to a field's name, type, or semantics requires a new version
and a regenerated golden fixture.

### Diagnostics fields

| Field             | Stable since | Meaning                                                          |
| ----------------- | ------------ | ---------------------------------------------------------------- |
| `tool`            | v0.1         | Tool name from the request.                                      |
| `envelopeVersion` | v0.1         | Echoes the constant `EnvelopeVersion`.                           |
| `sessionId`       | v0.2         | User session id (Phase 5). Empty for one-shot calls.             |
| `cdpSessionId`    | v0.2         | CDP flatten session id assigned by `Target.attachToTarget`.      |
| `targetId`        | v0.1         | Selected CDP target id.                                          |
| `endpoint`        | v0.1         | Resolved CDP endpoint (HTTP if probed, WS if direct).            |
| `cdpRoundTrips`   | v0.2         | Count of CDP commands during this call. Drops on session reuse.  |
| `durationMs`      | v0.1         | Wall-clock duration of the call.                                 |

### Error categories

| Category       | Used when                                                                  | Retryable?         |
| -------------- | -------------------------------------------------------------------------- | ------------------ |
| `validation`   | Params failed JSON Schema validation, or a tool-internal sanity check.     | No                 |
| `not_found`    | Unknown tool; no matching target / worksheet / range; selector not found.  | Sometimes          |
| `timeout`      | Per-call deadline exceeded.                                                | Yes                |
| `connection`   | CDP transport failure, session acquire failure, reconnect budget exhausted.| Yes                |
| `protocol`     | CDP responded with an error frame; payload threw outside its wrapper.      | No                 |
| `unsupported`  | Capability gates: dangerous CDP method without `--allow-dangerous-cdp`, Office requirement set unmet, etc. | No |
| `office_js`    | Office.js payload threw inside `Excel.run`. `error.details.debugInfo` has Excel's debugInfo. | No |
| `internal`     | Bug in our code (marshal failure, decode failure, programmer error).       | No                 |

`error.retryable` is the dispatcher's hint to the agent. Categories that
can transiently fail (`connection`, `timeout`) set it to `true`.

## Tool catalog

Every tool's full JSON Schema is available at runtime via `office-addin-mcp
list-tools`. The summary below is for orientation only.

### Raw CDP surface — gated by `--expose-raw-cdp`

The default `tools/list` advertises only the high-level Office add-in
tools (`addin.*`, `pages.*`, `page.*`, `excel.*`). Pass
`--expose-raw-cdp` (or set `OFFICE_ADDIN_MCP_EXPOSE_RAW_CDP=1`) to
also register the hand-written `cdp.selectTarget` primer and the ~411
code-generated `cdp.<domain>.<method>` tools described below.

The legacy aliases `cdp.evaluate`, `cdp.getTargets`, and
`browser.navigate` were removed in Phase 6. Their successors are:

| Removed             | High-level replacement | Raw CDP equivalent      |
| ------------------- | ---------------------- | ----------------------- |
| `cdp.evaluate`      | `page.evaluate`        | `cdp.runtime.evaluate`  |
| `cdp.getTargets`    | `pages.list`           | `cdp.target.getTargets` |
| `browser.navigate`  | `page.navigate`        | `cdp.page.navigate`     |

`cdp.selectTarget` has no direct CDP analogue; it primes the
per-session selector cache. High-level callers should prefer
`pages.select`.

### Code-generated `cdp.<domain>.<method>` (~411 tools)

Every CDP method in [`cdp/manifest.yaml`](../cdp/manifest.yaml) is
exposed as a tool with the canonical name shape
`cdp.<lowerDomain>.<lowerMethod>` (regex
`^cdp\.[a-z][a-zA-Z]*\.[a-z][a-zA-Z]*$`, enforced by
`internal/tools/cdptool/naming_test.go`).

The full domain-grouped index lives in
[`docs/cdp-tools.md`](./cdp-tools.md). Highlights:

- **Result shape:** `data` is the raw `Runtime.evaluate` /
  `Page.navigate` / etc. response from Chrome, passed through verbatim
  as `json.RawMessage`. Field names match the protocol JSON. A Chrome
  protocol roll is therefore a breaking change at the result-shape
  boundary — see [migration-notes.md](migration-notes.md).
- **Selectors:** target-scoped tools (everything outside
  `Browser.*`/`Target.*`/`Storage.*`/`CacheStorage.*`/`BackgroundService.*`)
  accept optional `targetId` / `urlPattern`. Empty selector falls back
  to "first non-internal page".
- **Auto-enable:** `Page`, `Runtime`, `DOM`, `CSS`, `Network`, `Fetch`,
  `Debugger`, `Animation`, `WebAuthn`, `Accessibility` issue
  `<Domain>.enable` once per CDP session before their first command.
  The cost is hidden from callers; you'll see one extra
  `cdpRoundTrips` on the first call into a domain.
- **Excluded:** `<Domain>.enable` / `.disable` are dispatcher-managed.
  `Target.attachToTarget`, `Target.setAutoAttach`,
  `Target.detachFromTarget`, `Target.autoAttachRelated`,
  `Target.attachToBrowserTarget`, `Target.exposeDevToolsProtocol`,
  `Target.sendMessageToTarget` are reserved for the dispatcher and
  never exposed.

### Dangerous-method gating

A small set of generated tools is marked `dangerous: true` in the
manifest:

- `cdp.browser.crash`, `cdp.browser.crashGpuProcess`,
  `cdp.browser.close`, `cdp.browser.executeBrowserCommand`
- `cdp.page.crash`
- `cdp.runtime.terminateExecution`
- `cdp.debugger.pause`
- `cdp.network.clearBrowserCache`, `cdp.network.clearBrowserCookies`
- `cdp.storage.clearCookies`, `cdp.storage.clearDataForOrigin`,
  `cdp.storage.clearDataForStorageKey`

These refuse with
`{ok:false, error:{category:"unsupported", code:"dangerous_disabled"}}`
unless the dispatcher was started with `--allow-dangerous-cdp` or
`OAMCP_ALLOW_DANGEROUS_CDP=1`. The flag is process-wide on `call` /
`serve` / `daemon`; per-call override is not supported.

### Binary `outputPath` for screenshots, PDFs, MHTML snapshots

Three generated tools accept an optional `outputPath` and decode their
base64 result to disk when it's set:

| Tool                          | Binary field | MIME type            |
| ----------------------------- | ------------ | -------------------- |
| `cdp.page.captureScreenshot`  | `data`       | `image/png`          |
| `cdp.page.printToPDF`         | `data`       | `application/pdf`    |
| `cdp.page.captureSnapshot`    | `data`       | `multipart/related`  |

When `outputPath` is set, the envelope returns:

```json
{ "path": "/abs/or/relative/path.png", "sizeBytes": 12345, "mimeType": "image/png" }
```

instead of the raw `{data: "<base64>", ...}` passthrough. Parent
directories are created if missing; existing files are overwritten.
Output failures map to `category=internal` (path/write errors) or
`category=protocol` (CDP didn't return the expected base64 field).

### `page.consoleLog` / `page.networkLog` / `page.networkBody`

These are event-buffer tools. The first call against a target subscribes to
the relevant CDP events (`Runtime.consoleAPICalled` /
`Runtime.exceptionThrown` / `Log.entryAdded` for console;
`Network.requestWillBeSent` / `responseReceived` / `loadingFinished` /
`loadingFailed` for network) and starts a goroutine that drains them into
a per-target ring buffer kept on the session. Subsequent calls drain the
buffer.

Key behaviors:

- **Scope is per-target.** Each `cdpSessionId` gets its own ring buffer;
  switching pages with `pages.select` preserves the previous target's
  buffer so you can flip back.
- **Auto-start, no explicit start/stop tool.** Events fired before the
  first call are not captured — same constraint CDP itself imposes via
  the `enable` commands.
- **Cursor-based drain.** Each record has a monotonic `seq`. Pass the
  previous response's `lastSeq` as `sinceSeq` to read only new entries.
- **Bounded ring, default 1000 entries.** Override with `maxBuffer`
  (resizes existing buffer; shrinking drops oldest). `dropped: true` in
  the response means the cursor predates the oldest retained entry —
  some events were lost.
- **Cleared on CDP reconnect.** Buffers and pump goroutines are torn
  down whenever the underlying socket is replaced (a reconnect
  invalidates `cdpSessionId` anyway).

`page.networkLog` emits one record per **completed** request, correlated
across `requestWillBeSent` / `responseReceived` / `loadingFinished`. In-
flight requests are not visible until they finalize. Failed requests
appear with `failed: true` and an `errorText`. Headers are omitted by
default; pass `includeHeaders: true` to include them.

`page.networkBody` fetches the response body for a `requestId` taken
from a `page.networkLog` record. Hard-capped at 5 MiB; for larger bodies
or streaming, use `cdp.network.streamResourceContent` (requires
`--expose-raw-cdp`).

### `excel.*`

All Excel tools take `targetId` or `urlPattern` (typically pointing at
the WebView2 taskpane URL). `sheet` defaults to the active worksheet.

Read tools that materialize a 2D grid (`excel.activeRange`, `excel.usedRange`,
`excel.rangeProperties`, `excel.rangeFormulas`, `excel.tableRows`,
`excel.pivotTableValues`) cap output at 1000 cells: when exceeded, only the
top-left cell is returned and `truncated: true` flags the trim.

| Tool                            | Required params                                                | Notes                                                          |
| ------------------------------- | -------------------------------------------------------------- | -------------------------------------------------------------- |
| `excel.workbookInfo`            | —                                                              | Name, dirty/readonly flags, calc mode/state, protection.       |
| `excel.calculationState`        | —                                                              | Mode + state + engine version + iterativeCalculation.          |
| `excel.listNamedItems`          | —                                                              | Workbook-scoped names: type, value, formula, visibility.       |
| `excel.customXmlParts`          | —                                                              | Custom XML parts: id + namespace URI.                          |
| `excel.settingsGet`             | — (optional `key`)                                             | `Office.context.document.settings`. Single key or all.         |
| `excel.listWorksheets`          | —                                                              | Name, id, position, visibility, tab color, active flag.        |
| `excel.getActiveWorksheet`      | —                                                              |                                                                |
| `excel.worksheetInfo`           | — (optional `sheet`)                                           | Used range, protection, gridlines, tab color, dimensions.      |
| `excel.activateWorksheet`       | `name`                                                         | Requires ExcelApi 1.7.                                         |
| `excel.createWorksheet`         | `name`                                                         |                                                                |
| `excel.deleteWorksheet`         | `name`                                                         | Excel may protect the active or last-visible sheet.            |
| `excel.listComments`            | — (optional `sheet`)                                           | Author, content, date, address, threaded replies.              |
| `excel.listShapes`              | — (optional `sheet`)                                           | Shapes/images: type, position, size, alt text.                 |
| `excel.readRange`               | `address`                                                      | Returns values, formulas, numberFormat, shape.                 |
| `excel.writeRange`              | `address` + one of `values`/`formulas`/`numberFormat` (anyOf)  |                                                                |
| `excel.getSelectedRange`        | —                                                              |                                                                |
| `excel.setSelectedRange`        | `address`                                                      |                                                                |
| `excel.activeRange`             | — (optional `includeFormulas`, `includeNumberFormat`)          | Selection with optional formulas / number formats. Truncates.  |
| `excel.usedRange`               | — (optional `sheet`, `valuesOnly`, include flags)              | Used range with truncation.                                    |
| `excel.rangeProperties`         | — (optional `address`, `sheet`, `includeFormat`, `includeStyle`) | Value types, hidden flags, font/fill/alignment, named style. |
| `excel.rangeFormulas`           | — (optional `address`, `sheet`)                                | Values + A1 + R1C1 formulas.                                   |
| `excel.rangeSpecialCells`       | `cellType` (+ optional `valueType`)                            | constants/formulas/blanks/visible. Returns address + count.    |
| `excel.findInRange`             | `text` (+ optional `completeMatch`, `matchCase`)               | findAll over a range. Requires ExcelApi 1.9.                   |
| `excel.listConditionalFormats`  | — (optional `address`, `sheet`)                                | Falls back to active sheet's used range.                       |
| `excel.listDataValidations`     | — (optional `address`, `sheet`)                                | type, rule, errorAlert, prompt, valid.                         |
| `excel.createTable`             | `address` (+ optional `name`, `hasHeaders`)                    |                                                                |
| `excel.listTables`              | —                                                              | All ListObjects with name, worksheet, address, row count, style. |
| `excel.tableInfo`               | `name`                                                         | Columns, filter criteria, header/total flags.                  |
| `excel.tableRows`               | `name` (+ optional `includeHeaders`)                           | Data-body values, truncated to cell cap.                       |
| `excel.tableFilters`            | `name`                                                         | Active filter criteria per column.                             |
| `excel.listCharts`              | — (optional `sheet`)                                           | Charts across worksheets: type, title, position, size.         |
| `excel.chartInfo`               | `sheet`, `name`                                                | Title, axis titles, series.                                    |
| `excel.chartImage`              | `sheet`, `name` (+ optional `width`, `height`)                 | PNG via Office.js. Returned as MCP `ImageContent`.             |
| `excel.listPivotTables`         | —                                                              | Name, worksheet, layout address, enabled flags.                |
| `excel.pivotTableInfo`          | `name`                                                         | Row/column/data/filter hierarchies.                            |
| `excel.pivotTableValues`        | `name`                                                         | Layout values, truncated to cell cap.                          |
| `excel.runScript`               | `script` (+ optional `scriptArgs`)                             | Permissive — runs an arbitrary async body inside `Excel.run`. |

`excel.runScript`'s body sees `context` (RequestContext) and `args`
(your `scriptArgs` JSON). Return any JSON-serializable value. Example:

```json
{
  "tool": "excel.runScript",
  "params": {
    "script": "const w = context.workbook; w.load('name'); await context.sync(); return {workbook: w.name};",
    "urlPattern": "taskpane"
  }
}
```

## Schema conventions

Every tool schema declares `additionalProperties: false`, so typos in
param names are caught at the dispatcher boundary before the tool runs.
Use `office-addin-mcp list-tools | jq '.tools[] | select(.name=="excel.readRange").schema'`
to see the canonical schema for any tool.

## Stability guarantees for v0.1

- The envelope shape is stable; new optional fields may appear without
  bumping `envelopeVersion`. Existing fields will not change type or
  semantics without a bump.
- Tool names, required params, and error categories are stable.
- Optional params, schema descriptions, and tool descriptions may be
  refined without a version bump.
- `error.code` strings are stable for the categories `validation`,
  `not_found`, `timeout`, `connection`, `internal`. `office_js` codes
  pass through whatever Excel reports (e.g. `ItemNotFound`,
  `InvalidArgument`) — stable as far as Excel is stable.

## Daemon HTTP API

When `call` autoroutes to a running daemon, the wire request is:

```
POST /v1/call
Authorization: Bearer <token>
Content-Type: application/json

{
  "tool":      "...",
  "params":    { ... },
  "sessionId": "default",
  "endpoint":  { "wsEndpoint": "...", "browserUrl": "..." },
  "timeoutMs": 30000
}
```

Response is the envelope, status 200. Auth failures return 401 with
`{"error":"unauthorized"}`. The token is in the daemon's socket file;
see [README.md](../README.md#daemon-mode).

Other endpoints:

- `GET /v1/health` — auth-free, returns `{"ok":true,"envelopeVersion":"v0.2"}`.
- `GET /v1/list-tools` — same shape as the `list-tools` subcommand.
- `GET /v1/status` — `{envelopeVersion, sessions: [id, ...]}`.
