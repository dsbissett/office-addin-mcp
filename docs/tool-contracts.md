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
| `unsupported`  | Reserved for capability gates (Office requirement set unmet, etc.).        | No                 |
| `office_js`    | Office.js payload threw inside `Excel.run`. `error.details.debugInfo` has Excel's debugInfo. | No |
| `internal`     | Bug in our code (marshal failure, decode failure, programmer error).       | No                 |

`error.retryable` is the dispatcher's hint to the agent. Categories that
can transiently fail (`connection`, `timeout`) set it to `true`.

## Tool catalog

Every tool's full JSON Schema is available at runtime via `office-addin-mcp
list-tools`. The summary below is for orientation only.

### `cdp.*`

| Tool                | Required params  | Returns                                                              |
| ------------------- | ---------------- | -------------------------------------------------------------------- |
| `cdp.evaluate`      | `expression`     | `{type, value, description}` — `Runtime.evaluate` result by value.   |
| `cdp.getTargets`    | —                | `{targets: TargetInfo[]}` — page/iframe/worker entries; chrome:// stripped by default. |
| `cdp.selectTarget`  | `targetId` or `urlPattern` (anyOf) | `{target: TargetInfo}` — primes the session selector cache. |

Common optional params on every CDP-backed tool: `targetId`,
`urlPattern`. Empty selector falls back to "first non-internal page".

### `browser.*`

| Tool             | Required params | Returns                                  |
| ---------------- | --------------- | ---------------------------------------- |
| `browser.navigate` | `url`         | `{frameId, loaderId, url}`. Surfaces `Page.navigate.errorText` as `category=protocol`. |

### `excel.*`

All Excel tools take `targetId` or `urlPattern` (typically pointing at
the WebView2 taskpane URL). `sheet` defaults to the active worksheet.

| Tool                          | Required params                              | Notes                                          |
| ----------------------------- | -------------------------------------------- | ---------------------------------------------- |
| `excel.readRange`             | `address`                                    | Returns values, formulas, numberFormat, shape. |
| `excel.writeRange`            | `address` + one of `values`/`formulas`/`numberFormat` (anyOf) |                            |
| `excel.listWorksheets`        | —                                            | name, id, position, visibility per sheet.      |
| `excel.getActiveWorksheet`    | —                                            |                                                |
| `excel.activateWorksheet`     | `name`                                       | Requires ExcelApi 1.7.                         |
| `excel.createWorksheet`       | `name`                                       |                                                |
| `excel.deleteWorksheet`       | `name`                                       | Excel may protect the active or last-visible sheet. |
| `excel.getSelectedRange`      | —                                            |                                                |
| `excel.setSelectedRange`      | `address`                                    |                                                |
| `excel.createTable`           | `address` (+ optional `name`, `hasHeaders`)  |                                                |
| `excel.runScript`             | `script` (+ optional `scriptArgs`)           | Permissive — runs an arbitrary async body inside `Excel.run`. |

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
