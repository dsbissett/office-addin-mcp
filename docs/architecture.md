# Architecture

A short tour of the packages and data flow. For background on what was
ported and why, see [migration-notes.md](migration-notes.md). For the
on-the-wire envelope, see [tool-contracts.md](tool-contracts.md).

## Layered package map

```
cmd/office-addin-mcp/main.go         — flag parse, dispatch to subcommand
cmd/gen-cdp-tools/                   — code generator: manifest+protocol → Go tools
  main.go        CLI: -manifest -browser -js -out -docs
  manifest.go    YAML loader (cdp/manifest.yaml)
  protocol.go    browser_protocol.json / js_protocol.json loader
  schema.go      CDP type → JSON Schema, $ref resolution, selector/outputPath fields
  template.go    text/template-driven Go rendering (gofmt in-process)
  golden_test.go fixture-based golden + determinism tests
  drift_test.go  re-runs against live manifest; fails if checked-in output drifts

cdp/                                 — vendored protocol + policy overlay
  manifest.yaml              policy: allowlist, scope, autoEnable, dangerous, binaryField
  protocol/browser_protocol.json     vendored Chrome devtools-protocol JSON
  protocol/js_protocol.json          vendored js_protocol JSON
  protocol/VERSION                   pinned upstream SHA + refresh recipe
  protocol_test.go                   sanity test: parses, skeleton domains present
scripts/build_manifest.py            regenerates manifest.yaml from CdpProtocols.md

internal/cli/                        — call, list-tools, daemon, serve
  call.go        flag parse, daemon autoroute, in-process fallthrough
  list_tools.go  list registered tools as JSON
  daemon.go      run the persistent TCP server (foreground)
  serve.go       --stdio: newline-delimited JSON in/out
  dangerous.go   shared --allow-dangerous-cdp flag + OAMCP_ALLOW_DANGEROUS_CDP env
  registry.go    DefaultRegistry — wires tool packages

internal/tools/                      — registry, dispatcher, envelope
  registry.go    Tool, Registry; schemas compiled at registration
  dispatcher.go  Dispatcher{Registry, Sessions, AllowDangerous} — validate → run → finalize
  result.go      Envelope, EnvelopeError, Diagnostics, EnvelopeVersion
  runtime.go     Request, Result, RunEnv (Conn / Attach / EnsureEnabled / AllowDangerous)
  binary.go      WriteBinaryFieldOutput — base64 → disk for binary tools
  schema.go      JSON Schema validation (santhosh-tekuri/jsonschema/v5)
  target.go      ResolveTarget, IsInternalURL
  cdptool/       Register: cdp.selectTarget primer + generated.RegisterGenerated
                 for ~411 tools. Gated by --expose-raw-cdp; the default registry
                 omits this package.
    generated/   one Go file per CDP domain (accessibility.go … webauthn.go);
                 emitted by gen-cdp-tools, byte-identical across runs
  exceltool/     37 excel.* tools — Office.js payloads via officejs

internal/session/                    — Phase-5 session pool
  session.go     Session: lock + conn + reconnect budget + selector cache
  manager.go     Manager: pool + optional idle GC

internal/daemon/                     — Phase-5 HTTP server
  server.go      bearer auth, /v1/{health,call,list-tools,status}
  client.go      Probe + CallDaemon, used by call autoroute
  socket.go      well-known socket file (port + token + pid)

internal/officejs/                   — Office.js execution
  executor.go    wrap preamble + payload + args, evaluate, unwrap result
  payloads.go    embed loader, file→tool name mapping, @requires parsing

internal/js/                         — embedded *.js sources
  embed.go       //go:embed all:*.js
  _preamble.js   __officeError, __ensureOffice, __requireSet, __runExcel
  excel_*.js     payload file per excel.* tool (one-to-one with tool name)

internal/cdp/                        — CDP WebSocket protocol
  connection.go  ws dial, message pump, request/response correlation,
                 RoundTrips counter, event subscribe
  target.go      getTargets, attachToTarget (flatten), createTarget
  runtime.go     Runtime.evaluate, exception unwrap
  page.go        Page.navigate
  discovery.go   /json/version probe

internal/webview2/                   — endpoint discovery policy
  discover.go    ws-endpoint > browser-url > default :9222 > OS scan
  scan_windows.go / scan_other.go   Windows scan stubbed for v0.1
```

The arrows always go down: `cli` may import `tools`, `tools` may import
`session` and `cdp`, `session` may import `cdp` and `webview2`. Nothing
upstream imports anything downstream from itself. `cdp` and `webview2`
are the leaf protocol packages.

## Data flow — a single tool call

```
office-addin-mcp call --tool excel.readRange --param '{"address":"A1"}'
   ↓
cli/call.go RunCall
   ↓ (probe socket file; healthy daemon? → HTTP POST /v1/call → done)
   ↓ (else)
tools.Dispatch(ctx, registry, request)        // free function
   = Dispatcher{Sessions: ephemeral mgr, Ephemeral: true}
   ↓
Dispatcher.Dispatch
   1. registry.Get(tool)
   2. validateParams against tool.Schema
   3. session.Manager.Get(req.SessionID)      // may create
   4. session.Acquire(ctx, endpoint)          // lock + ensure conn
   5. tool.Run(ctx, params, RunEnv{Conn,Attach})
   6. finalize envelope (CDPRoundTrips, DurationMs, EnvelopeVersion)
   7. release session (one-shot: Drop)
```

In daemon mode, step 3's session persists across requests; the lock is
held only for the duration of one tool call so different sessions can
serve concurrent requests.

## RunEnv — what tools see

Tools never manage connection lifetimes. They get a `*RunEnv` whose two
helpers do the right thing for either mode:

```go
type RunEnv struct {
    Diag *Diagnostics
    Conn           func(ctx) (*cdp.Connection, error)
    Attach         func(ctx, TargetSelector) (*AttachedTarget, error)
    EnsureEnabled  func(ctx, cdpSessionID, domain string) error
    AllowDangerous bool
}
```

- `Conn` is for tools that don't need to attach (`cdp.target.getTargets`,
  `cdp.browser.getVersion`).
- `Attach` resolves a target and attaches via flatten sessions; in
  daemon mode it consults the session's **selector cache** so repeat
  calls with the same `(targetId, urlPattern)` skip both
  `Target.getTargets` and `Target.attachToTarget`. The daemon
  acceptance test verifies this — `diagnostics.cdpRoundTrips` drops
  from ~3 to 1 after the first call.
- `EnsureEnabled` issues `<Domain>.enable` exactly once per
  `(cdpSessionID, domain)` pair on the active session. Generated
  CDP tools call this before the first command on any auto-enable
  domain (Page, Runtime, DOM, CSS, Network, Fetch, Debugger,
  Animation, WebAuthn, Accessibility). The bookkeeping clears on
  reconnect — Chrome resets domain state across connections, and
  `Session.dropConnLocked` follows.
- `AllowDangerous` is the per-process gate fed by
  `--allow-dangerous-cdp` / `OAMCP_ALLOW_DANGEROUS_CDP`. Generated
  tools whose manifest entry has `dangerous: true` (Browser.crash,
  Runtime.terminateExecution, etc.) refuse with
  `category=unsupported, code=dangerous_disabled` when this is false.

`AttachedTarget` is a value (no Close method); the dispatcher owns the
underlying connection.

## Session lifecycle

A `*session.Session` holds:

- One `cdp.Connection` (lazily dialed on first `Acquire`)
- The endpoint config used to dial (re-dial on change)
- A sticky selection cache: `(selectorTargetID, selectorURLPattern) →
  (TargetInfo, cdpSessionID)`
- A sliding-window reconnect budget (default 3 in 60s)
- A mutex serializing tool calls on the session

State transitions:

```
new → first Acquire → dial → ready
                          ↓
                       hand to Run
                          ↓
                       release (unlock; conn stays)
                          ↓
ready → next Acquire → liveness check → ready
                              ↓ (Done() fired)
                          re-dial (if budget allows)

idle past IdleTimeout → Manager.gcOnce → Close (drop conn)
explicit Manager.Drop  → Close
```

Failed dials count against the budget too — repeated misses against an
unreachable endpoint surface as `connection / session_acquire_failed`
once the budget is exhausted.

## Office.js boundary

Each `excel.*` tool is a thin Go shim that:

1. Decodes typed params + a `(targetId, urlPattern)` selector.
2. Calls `env.Attach(ctx, selector)` to get
   `AttachedTarget{Conn, Target, SessionID}`.
3. Hands them to `officejs.Executor.Run(ctx, toolName, args)`.

`Executor.Run` builds the JS expression to evaluate as:

```js
(async (args) => {
  try {
    /* preamble — __officeError, __ensureOffice, __requireSet, __runExcel */
    /* payload body — return { result: ... } */
  } catch (e) {
    if (e && e.__officeError) {
      return { __officeError: true, code, message, debugInfo };
    }
    return { __officeError: true, code: 'unhandled_exception', ... };
  }
})(<argsJSON>)
```

`encoding/json`'s default Marshal already escapes U+2028/U+2029, so the
embedded `<argsJSON>` is JS-source-safe.

The result envelope from the JS side is exactly two shapes:

- `{ "result": <user-data> }` — payload returned successfully
- `{ "__officeError": true, "code": "...", "message": "...", "debugInfo":
  ... }` — anything that threw inside the wrapper

The Go executor unwraps the first into the tool's `data`, the second
into a `category=office_js` envelope error with `debugInfo` tucked into
`error.details`.

## Code generation — where the ~411 cdp.* tools come from

Two artifacts are the source of truth:

1. **Vendored protocol JSON** — `cdp/protocol/{browser,js}_protocol.json`,
   pinned to the SHA recorded in `cdp/protocol/VERSION`. These are the
   parameter schemas, type definitions, and descriptions Chrome publishes.
2. **Manifest** — `cdp/manifest.yaml`, regenerated by
   `scripts/build_manifest.py`. Carries policy only: which methods to
   expose, scope (browser vs target), autoEnable per-domain, plus
   `dangerous`/`binaryField`/`binaryMimeType` annotations.

`go generate ./...` (declared in
[`internal/tools/cdptool/generated/doc.go`](../internal/tools/cdptool/generated/doc.go))
runs `cmd/gen-cdp-tools` which:

1. Loads the manifest and the two protocol JSON files.
2. For every manifest method, derives a JSON Schema by walking
   `parameters` and resolving `$ref`s against the protocol's
   `domain.types` (cross-domain via `Domain.Type`).
3. Renders one Go file per CDP domain (`internal/tools/cdptool/generated/<domain>.go`)
   with: a schema constant, a params struct, an exported
   `New<Domain><Method>` factory, and an unexported `run<Domain><Method>`
   function. Target-scoped tools project params into a CDP-only struct
   (no selectors / outputPath). Auto-enable domains emit
   `env.EnsureEnabled` before the first command. Dangerous methods
   prepend the `env.AllowDangerous` guard. Binary-field methods append
   the `outputPath` branch.
4. Emits `register_generated.go` aggregating each domain's
   `Register<Domain>(r)` into one `RegisterGenerated(r)`.
5. Emits `docs/cdp-tools.md` — a domain-grouped index.

### Determinism

Every map iteration sorts keys; `go/format` runs in-process; output is
byte-stable across runs. `cmd/gen-cdp-tools/drift_test.go` re-runs the
generator against the live manifest into a tempdir and byte-compares to
the checked-in copies — it's the test-suite equivalent of
`go generate ./... && git diff --exit-code`.

### What's *not* generated

- `cdp.selectTarget` stays hand-written — no CDP equivalent exists; it
  primes the per-session selector cache. Like the rest of the `cdp.*`
  surface it is hidden by default and only registered with
  `--expose-raw-cdp`.
- The legacy aliases `cdp.evaluate`, `cdp.getTargets`, and
  `browser.navigate` were removed in Phase 6. Use the high-level
  replacements (`page.evaluate`, `pages.list`, `page.navigate`) or, if
  raw CDP is needed, `cdp.runtime.evaluate` / `cdp.target.getTargets` /
  `cdp.page.navigate` under `--expose-raw-cdp`.

## What lives where (rules of thumb)

- **Protocol fact:** `internal/cdp/`. Adding `Page.bringToFront`? It
  belongs here.
- **Discovery policy:** `internal/webview2/`. The "where is the browser?"
  question — endpoint priority, OS scans.
- **Session lifecycle / connection pooling:** `internal/session/`.
- **Tool surface for agents:** `internal/tools/<domain>tool/`. The
  schemas live here, next to the Go code that implements the tool.
- **Office.js semantics:** `internal/js/*.js` (the body) +
  `internal/officejs/` (the runtime around it). The JS files are the
  source of truth for Excel semantics; Go just transports.
- **Daemon plumbing:** `internal/daemon/` (HTTP, auth, socket file).
  Pure transport — no tool logic.
- **CLI:** `internal/cli/`. Parses flags, picks a transport (daemon
  autoroute / in-process / stdio), prints the envelope.
