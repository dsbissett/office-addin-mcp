# Changelog

## Unreleased

### Changed

- **Multi-host F9 — `--launch-addin` flag (host-agnostic) with
  `--launch-excel` kept as a deprecated alias.** The boot-time
  auto-launch flag was named `--launch-excel`, which read as
  Excel-specific even though the underlying `launch.LaunchIfNeeded`
  has been host-agnostic since F3. Reasoning: rename to a name that
  matches reality, but don't break scripts and `mcp.json` snippets
  pinned to the old flag.
  - `cmd/office-addin-mcp/main.go` — added `--launch-addin` flag.
    `--launch-excel` is kept and now documents itself as a "Deprecated
    alias for --launch-addin." The boot guard fires when *either*
    flag is set (`*launchAddin || *launchExcel`). Renamed
    `autoLaunchExcel` → `autoLaunchAddin`; the body is unchanged
    because `launch.DetectAddin` already accepts any Office host (F3).
    The two `slog.{Warn,Info}` lines that mention the flag now name
    `--launch-addin` so log readers see the new spelling. `writeUsage`
    lists both flags with the alias clearly labelled deprecated.

- **Multi-host F8 — extended `StandardRequirementSets` for
  Word/Outlook/PowerPoint/OneNote.** `addin.contextInfo` only probed
  Excel and shared-runtime sets, so an agent calling it from a Word
  taskpane saw a misleading "no Word capabilities" report even when
  WordApi 1.4 was actually supported. Reasoning: probing 13 extra sets
  costs roughly nothing per call (`Office.context.requirements.isSetSupported`
  is sync and local), and an agent looking at one host should see that
  host's capability ladder.
  - `internal/addin/requirements.go` — appended 13 entries to
    `StandardRequirementSets`: WordApi 1.1/1.2/1.3/1.4, Mailbox
    1.1/1.5/1.8/1.10/1.13, PowerPointApi 1.1/1.2/1.3, OneNoteApi 1.1.
    Versions chosen to bracket each host's published API ladder so the
    `supported / unsupported` cutover line tells the agent how modern
    the runtime is.

### Added

- **Multi-host F7 — OneNote add-in tool surface (6 tools) + bulk
  registry wiring.** Lands the final host package and flips the full
  multi-host surface on by wiring `wordtool`, `outlooktool`,
  `powerpointtool`, and `onenotetool` into
  `internal/mcp/registry.go` together. Reasoning: keeping the
  registry wiring as the very last commit means F4/F5/F6/F7 each
  shipped behind their own `register_test.go` smoke without disturbing
  the live `tools/list` until everything was ready — and a single
  commit to flip on 27 new tools is easier to revert than four
  scattered ones.
  - `internal/js/onenote_*.js` *(6 new payloads)* —
    `onenote_read_notebooks.js` (id + name per notebook),
    `onenote_read_sections.js` (active notebook → sections),
    `onenote_read_pages.js` (active section → pages),
    `onenote_read_page.js` (active page → title + content list),
    `onenote_add_page.js` (append page to active section),
    `onenote_run_script.js` (escape hatch via `OneNote.run`). All
    dispatch through `__runOneNote` (F2).
  - `internal/tools/onenotetool/runner.go` *(new package)* — package
    forwarder calling `officetool.RunPayload(..., "OneNote")` plus
    the same `arrayLen` / `stringField` / `emptySelectorParams`
    helpers as the other host packages.
  - `internal/tools/onenotetool/notebook.go` — constructors for the
    five non-script tools (`onenote.readNotebooks`,
    `onenote.readSections`, `onenote.readPages`, `onenote.readPage`,
    `onenote.addPage`).
  - `internal/tools/onenotetool/script.go` — `onenote.runScript`
    escape hatch wrapped with `__runOneNote`.
  - `internal/tools/onenotetool/register.go` — `Register(r)`.
  - `internal/tools/onenotetool/register_test.go` — asserts 6 tools
    register and every one has an embedded JS payload.
  - `internal/mcp/registry.go` — added imports for the four new host
    packages and called their `Register(r)` after `exceltool.Register(r)`.
    The high-level surface now ships `excel.*` (37) + `word.*` (8) +
    `outlook.*` (7) + `powerpoint.*` (6) + `onenote.*` (6) by default,
    with no flag required.
  - `internal/mcp/registry_test.go` — new
    `TestDefaultRegistryIncludesMultiHostSurface` asserts at least one
    tool from each of the five host packages registers on the default
    (no-CDP) registry shape.

- **Multi-host F6 — PowerPoint add-in tool surface (6 tools).** Adds
  the PowerPoint host package against the F1 runner. Like F4/F5, the
  package's `Register()` is exported but not yet wired into the live
  MCP registry; F7's bulk wiring step turns the full multi-host
  surface on at once. Reasoning: the slide / shape / selection
  read-out pattern is enough to build "explain this deck" / "summarize
  every slide" agent flows, while a single `addSlide` write plus the
  permissive `runScript` escape hatch covers the long tail of
  composition use-cases without bloating the surface.
  - `internal/js/powerpoint_*.js` *(6 new payloads)* —
    `powerpoint_read_presentation.js` (title + slideCount),
    `powerpoint_read_slides.js` (id + shape names per slide),
    `powerpoint_read_slide.js` (per-shape geometry on one slide;
    out-of-range index throws `powerpoint_slide_out_of_range`),
    `powerpoint_add_slide.js` (append blank slide, return id),
    `powerpoint_read_selection.js` (ids of currently selected slides),
    `powerpoint_run_script.js` (escape hatch via `PowerPoint.run`).
    All dispatch through `__runPowerPoint` (F2).
  - `internal/tools/powerpointtool/runner.go` *(new package)* —
    package forwarder `runPayloadSum` calling
    `officetool.RunPayload(..., "PowerPoint")`, plus the same
    `arrayLen` / `stringField` / `emptySelectorParams` helpers as the
    other host packages and a `numberField` helper used by the
    presentation-summary line.
  - `internal/tools/powerpointtool/presentation.go` — constructors
    for the five non-script tools (`powerpoint.readPresentation`,
    `powerpoint.readSlides`, `powerpoint.readSlide`,
    `powerpoint.addSlide`, `powerpoint.readSelection`).
  - `internal/tools/powerpointtool/script.go` —
    `powerpoint.runScript` escape hatch, mirroring `excel.runScript`
    but wrapped with `__runPowerPoint`.
  - `internal/tools/powerpointtool/register.go` — `Register(r)`
    exported but not yet called from `internal/mcp/registry.go`.
  - `internal/tools/powerpointtool/register_test.go` — asserts
    exactly 6 tools register and every one has an embedded JS payload.

- **Multi-host F5 — Outlook add-in tool surface (7 tools).** Adds the
  Outlook host package against the F1 runner. Outlook is the odd one
  out: it has no batched-context `<Host>.run` API, and most of its
  property accessors are still callback-shaped (`getAsync` /
  `setAsync`). Reasoning: a thin Promise-wrapper inside each payload
  keeps the Go-side dispatch contract identical to the other hosts —
  the runner just awaits whatever the payload returns. Like F4, the
  package is registered in its own `Register()` but not yet wired into
  the live MCP registry; F7's bulk wiring step lands the four host
  packages together.
  - `internal/js/outlook_*.js` *(7 new payloads)* —
    `outlook_read_item.js`, `outlook_get_body.js`,
    `outlook_set_body.js`, `outlook_get_subject.js`,
    `outlook_set_subject.js`, `outlook_get_recipients.js`,
    `outlook_run_script.js`. All call `__runOutlook(async mailbox =>
    { … })` (the F2 helper passes `Office.context.mailbox` straight
    through). Each callback API is wrapped in `new Promise((resolve,
    reject) => x.getAsync(…, r => r.status === 'succeeded' ? … : …))`
    so the body still reads as straight-line `await` code.
    `outlook_get_subject.js` and `outlook_get_recipients.js` detect
    compose-vs-read mode at runtime by sniffing whether the field
    exposes `getAsync` (compose) or is a plain string / array (read).
  - `internal/tools/outlooktool/runner.go` *(new package)* — package
    forwarder `runPayloadSum` calling `officetool.RunPayload(...,
    "Outlook")`, plus the same `arrayLen` / `stringField` /
    `emptySelectorParams` helpers as `wordtool`.
  - `internal/tools/outlooktool/mailbox.go` — constructors for the six
    mailbox tools (`outlook.readItem`, `outlook.getBody`,
    `outlook.setBody`, `outlook.getSubject`, `outlook.setSubject`,
    `outlook.getRecipients`).
  - `internal/tools/outlooktool/script.go` — `outlook.runScript`
    escape hatch. Differs from `excel.runScript` / `word.runScript` in
    two ways: the user body sees `mailbox` (not `context`) and there
    is no `Outlook.run` wrapper on the JS side.
  - `internal/tools/outlooktool/register.go` — `Register(r)` exported
    but not yet called from `internal/mcp/registry.go`.
  - `internal/tools/outlooktool/register_test.go` — asserts exactly 7
    tools register and every one has an embedded JS payload.

- **Multi-host F4 — Word add-in tool surface (8 tools).** Adds the
  first non-Excel host package against the new officetool runner. None
  of the new tools are wired into the registry yet — the `Register`
  call lands in F7's bulk wiring step so a single commit flips on
  all four host packages at once. Reasoning: keeping the host packages
  individually verifiable (each has its own `register_test.go` that
  panic-checks every schema and asserts every tool has a JS payload)
  lets us land them serially without disturbing the live MCP surface.
  - `internal/js/word_*.js` *(8 new payloads)* —
    `word_read_body.js`, `word_write_body.js`,
    `word_read_paragraphs.js`, `word_insert_paragraph.js`,
    `word_read_selection.js`, `word_search_text.js`,
    `word_read_properties.js`, `word_run_script.js`. All call
    `__runWord(async ctx => { … })` introduced by F2 and follow the
    same `// @requires WordApi <ver>` convention used by the Excel
    payloads (so the existing requirement-set tooling parses them
    automatically).
  - `internal/tools/wordtool/runner.go` *(new package)* — package-level
    `runPayloadSum` that forwards to `officetool.RunPayload(...,
    "Word")`, plus the small `arrayLen` / `stringField` /
    `emptySelectorParams` helpers mirrored from `exceltool`. The
    selector-base const re-exports `officetool.TargetSelectorBase` so
    schemas in this package keep the same string-concat shape as in
    `exceltool`.
  - `internal/tools/wordtool/document.go` — constructors for the seven
    document-scoped tools (`word.readBody`, `word.writeBody`,
    `word.readParagraphs`, `word.insertParagraph`,
    `word.readSelection`, `word.searchText`, `word.readProperties`).
    Tools that take only the selector embed `emptySelectorParams`;
    tools with extra fields embed `officetool.SelectorFields` directly.
  - `internal/tools/wordtool/script.go` — `word.runScript` escape
    hatch, mirroring `excel.runScript` (compiles the user's body via
    `new Function(...)` and runs it inside `__runWord`).
  - `internal/tools/wordtool/register.go` — `Register(r)` exported but
    not yet called from `internal/mcp/registry.go`; F7 will flip the
    full multi-host surface on at once.
  - `internal/tools/wordtool/register_test.go` — mirrors the Excel
    register test: asserts exactly 8 tools register and every
    registered tool has an embedded JS payload.

### Changed

- **Multi-host F3 — generalized add-in manifest detection.**
  `launch.DetectAddin` only accepted manifests that declared the Excel
  Workbook host (XML manifests with `<Host Name="Workbook"/>` or JSON
  manifests with a `workbook` scope), so a Word / Outlook / PowerPoint /
  OneNote project would fail detection even though everything downstream
  is host-agnostic. Reasoning: the host-specific gate served no purpose
  beyond Phase-0 scoping — the launcher, CDP wiring, and Office.js
  payload runner all care about presence of an add-in, not which host
  it targets — so widening the gate is the cheapest path to multi-host
  support.
  - `internal/launch/detect.go` — renamed `isWorkbookXMLManifest` →
    `isOfficeXMLManifest` and dropped the `<Host Name="Workbook">`
    regex check; presence of `<OfficeApp` is now sufficient. Renamed
    `isWorkbookJSONManifest` → `isOfficeJSONManifest` and accepted any
    non-empty `extensions[].requirements.scopes` (was hard-coded to
    `"workbook"`). Updated `ErrNoProject` message and the `Project`
    struct's doc comment to drop "Excel".
  - `internal/launch/detect_test.go` — flipped
    `TestDetectAddin_NonWorkbookXMLRejected` to
    `TestDetectAddin_NonWorkbookXMLAccepted`: a manifest with
    `<Host Name="Document"/>` (Word) is now a valid project. All other
    fixture-based tests pass unchanged because their `<Host
    Name="Workbook">` markup still satisfies the relaxed `<OfficeApp`
    check.
  - `internal/tools/lifecycletool/detect.go` — package doc and
    `addin.detect` tool description no longer say "Excel" or
    "workbook-scoped"; the description now lists the supported hosts
    (Workbook, Document, Mailbox, Presentation, Notebook) explicitly so
    agents can tell at a glance the tool isn't Excel-only.

- **Multi-host F2 — generalized Office.js preamble for all hosts.**
  `internal/js/_preamble.js` only exposed `__runExcel` and the
  `__ensureOffice` gate hard-failed when `globalThis.Excel` was
  unavailable — fine for Excel-only payloads, but a non-starter for
  the upcoming Word / Outlook / PowerPoint / OneNote tools that need
  to run in taskpanes hosted by other Office apps. Reasoning: every
  host shares the same `Office.onReady` bootstrap; only the per-host
  `<Host>.run` wrapper differs, so it's cheaper to add four sibling
  helpers than to duplicate the readiness dance per host.
  - `internal/js/_preamble.js` — removed the `globalThis.Excel`
    presence check from `__ensureOffice`; the `office_unavailable`
    error now covers every host. Added `__runWord(fn)`,
    `__runPowerPoint(fn)`, `__runOneNote(fn)`, and `__runOutlook(fn)`
    alongside `__runExcel`. The first three follow the existing
    `Excel.run` pattern; `__runOutlook` is the odd one — it skips
    `<Host>.run` (Outlook has no equivalent batched-context API) and
    just hands `Office.context.mailbox` to the payload after readiness.
  - `internal/tools/addintool/errors.go` — `recoveryHintForOfficeCode`
    no longer matches `excel_unavailable` (the preamble can no longer
    throw it); the `office_unavailable` hint is now phrased
    host-agnostically ("Office.js is not loaded in this target").
  - `internal/officejs/payloads_test.go` — `TestPreambleEmbedded`
    asserts the four new run helpers are concatenated into the
    embedded preamble alongside the existing markers.

- **Multi-host F1 — extracted shared payload runner into `internal/tools/officetool`.**
  The `runPayload` scaffolding (Attach → Office.js dispatch → envelope mapping)
  used to live entirely inside `internal/tools/exceltool/runner.go`, which
  meant adding Word/Outlook/PowerPoint/OneNote support would require copying
  the same ~50 lines four times. Reasoning: the runner is genuinely
  host-agnostic — only the host label embedded in failure summaries varies —
  so extracting it once unblocks every subsequent host package without
  forking the dispatch loop.
  - `internal/tools/officetool/runner.go` *(new)* — exports
    `TargetSelectorBase` (JSON-Schema fragment), `SelectorFields` (embedded
    params struct) with a `Selector()` method, and `RunPayload(ctx, env, sel,
    payload, args, summaryFn, hostLabel)`. The `hostLabel` parameter
    ("Excel" / "Word" / …) is interpolated into attach/payload-failure
    summaries so chat clients see which host's call broke.
  - `internal/tools/exceltool/runner.go` — `runPayloadSum` is now a
    one-line forwarder to `officetool.RunPayload(..., "Excel")`. Removed the
    now-redundant local `codeOrDefault` helper. Kept the unexported
    `selectorFields` / `targetSelectorBase` so existing per-tool call sites
    in this package compile unchanged.

- **Headless-Chrome integration test gated behind `-tags integration`.**
  `internal/cdp/integration_test.go` previously ran in non-`-short` mode
  and was the source of intermittent CI failures on Windows runners
  (the `DevToolsActivePort` wait raced with how Windows-Chrome wrote the
  file). Reasoning: this test is genuinely useful as a local smoke when
  Chrome is on the box, but it has no business gating CI green/red
  every time a runner image changes. Build-tagging it makes the
  default `go test ./...` skip it entirely while devs still get the
  coverage with one extra flag.
  - `internal/cdp/integration_test.go` — added a `//go:build integration`
    build tag at the top of the file plus a doc comment pointing at the
    `go test -tags integration ./internal/cdp/...` invocation. Removed
    the now-redundant `if testing.Short() { t.Skip(...) }` guard in
    `TestEvaluate_AgainstHeadlessChrome` since the build tag is now the
    single source of truth for whether the test runs.

### Added

- **Performance benchmarks for the dispatcher and session hot paths.**
  Until F9 the codebase had zero `Benchmark*` functions, so any perf
  regression in the dispatcher or session-locking refactor (F5) had to
  be detected by feel. Reasoning: a small set of micro-benchmarks
  covering the per-call floor (NoSession dispatch), the F5 contract
  (parallel Acquire), the F5 fast path (warm Acquire), and the inner
  hot-spots (selection cache + EnsureEnabled hit) gives a number to
  track without inventing fixtures. Concretely:
  - `internal/tools/dispatcher_bench_test.go` *(new)* — three benches
    that don't need a CDP connection: `BenchmarkDispatchNoSession`
    (lifecycle-tool floor: lookup + schema validate + request-id +
    finalize), `BenchmarkDispatchValidationError` (schema-violation
    fast-fail path), `BenchmarkMarshalEnvelope` (the JSON encode the
    adapter pays on every result, including the diagnostics block).
  - `internal/session/session_bench_test.go` *(new)* — four benches
    against the existing `fakeBrowser`: `BenchmarkSessionAcquireWarm`
    (steady-state RLock fast path), `BenchmarkSessionParallelAcquire`
    (validates F5 via `b.RunParallel`; pair with `-cpu=1,2,4,8` to
    chart scaling), `BenchmarkSelectionCacheHit` (stateMu + matches),
    `BenchmarkEnsureEnabledHit` (stateMu + nested map lookup).
  - `internal/session/session_test.go` — relaxed
    `newFakeBrowser(t *testing.T)` → `newFakeBrowser(t testing.TB)` so
    `*testing.B` reuses the same fixture without copy-paste.

  Smoke from a local run (Windows, i9-13900HX, GOMAXPROCS=32):
  `BenchmarkSessionAcquireWarm` ≈ 1.05 µs/op,
  `BenchmarkSessionParallelAcquire` ≈ 1.91 µs/op (slightly higher
  because parallel callers contend on the read-lock atomic, but still
  well below the old serial mutex), `BenchmarkSelectionCacheHit` ≈
  18 ns/op, `BenchmarkEnsureEnabledHit` ≈ 22 ns/op. These become the
  baseline regression guard once `.github/workflows/ci.yml` gets the
  follow-up `bench` job (deferred — out of scope for F9 since it
  requires a stable runner profile).

  Live-Office smoke (sample manifest under `testdata/sample-addin/`)
  and the `internal/officejs/executor.go` payload-latency stamps are
  the remaining F9 deliverables; both are deferred behind a fixture
  workbook that's not yet checked in. The structural pieces — opt-in
  integration tests + reusable benchmarks — ship now so a future PR
  can layer the live-Office fixture without re-architecting either.

### Changed

- **Concurrent CDP dispatch per session.** Previously
  `Session.Acquire` took a `sync.Mutex` for the *entire* tool-call
  duration, so an agent batching parallel calls (e.g.
  `excel.readRange` + `excel.listTables` + `page.screenshot`) ran them
  serially against the same session even though `cdp.Connection.Send`
  is already concurrent-safe (each request gets its own response
  channel keyed on the CDP id). Reasoning: for an "AI bridge" the
  natural workload is fan-out reads, and the lock was the only thing
  blocking near-linear speedup on independent CDP commands.
  Concretely:
  - `internal/session/session.go` — replaced the single `mu sync.Mutex`
    with three narrower locks:
    - `connMu sync.RWMutex` — guards the connection pointer +
      endpoint config + reconnect budget. Read-locked on the
      steady-state Acquire fast path so N goroutines share one dial;
      write-locked only on dial / reconnect / close.
    - `stateMu sync.Mutex` — guards the per-call sticky state
      (selection cache, default selection, snapshot, enabled-domain
      bookkeeping). Self-locked by `Selected` / `SetSelected` /
      `DefaultSelection` / `SetSnapshot` / etc. so callers no longer
      need to hold any "outer" lock.
    - `eventMu sync.Mutex` — guards the eventBufs map shape (the
      buffer values keep their own internal mutex).
    `Acquire` now uses the standard double-checked-locking pattern:
    RLock → fast path return; on a miss drop and Lock → recheck → dial
    → downgrade back to RLock for the caller's release. The release
    closure is once-only so a defensive double-call doesn't panic the
    underlying `sync.RWMutex`.
  - `internal/session/session.go` — `lastUsed time.Time` →
    `lastUsedNano atomic.Int64`, so `LastUsed()` (called by the
    manager's gc loop on every tick) is lock-free and doesn't contend
    with Acquire.
  - `internal/session/session.go` — `EnsureEnabled` no longer
    serializes the entire dispatch path on the one-shot
    `<Domain>.enable` call. Fast path: cheap `stateMu` check + return.
    Slow path: send the enable *outside* any session lock, then mark
    enabled. Concurrent first-callers may both Send — Chrome treats
    `<Domain>.enable` as idempotent in practice, so the duplicate is
    harmless and avoids a serial point that would defeat F5.
  - `internal/session/session.go` — `dropConnLocked` (called only with
    `connMu` write-locked) now briefly takes `stateMu` and `eventMu`
    to clear their respective maps. Lock order is enforced as
    connMu → stateMu → eventMu everywhere; nothing else takes locks
    in the reverse direction, so no deadlock is possible.
  - `internal/session/eventbuf.go` — `EventBuf` and
    `MarkEventPumping` are now self-locking on `eventMu`. The dead
    `dropEventBufsLocked` helper was inlined into `dropConnLocked`.
  - `internal/session/enable_test.go` — the reconnect-clears test
    used the old `s.mu`; updated to `s.connMu` to match the new shape.
  - `internal/session/session_test.go` — new
    `TestConcurrentAcquireDoesNotSerialize` runs 16 goroutines × 50
    Acquire/release cycles each (each holds the read lock for ~1ms to
    simulate a CDP send) and asserts `fakeBrowser.dialCount == 1`,
    proving 800 parallel acquires share one dial. Existing tests
    (`AcquireDialsOnceAndReuses`, `ReconnectBudgetExhaustion`,
    `StickySelectionCache`, `ReconnectClearsSelectionCache`,
    `SnapshotCache`, the EnsureEnabled trio) all pass unchanged
    against the new shape.

### Added

- **`--cdp-domains` flag for slicing the raw CDP surface.** When users
  flipped on `--expose-raw-cdp`, every code-generated `cdp.*` tool
  registered at once — ~411 tools cost the agent thousands of tokens in
  the `tools/list` response, and most domains (Animation, WebAuthn,
  Debugger) aren't useful for Office automation. Reasoning: agents that
  only need DOM + Page + Runtime should be able to opt into that slice
  without paying for the rest, and a typo in `--cdp-domains` should fail
  fast with a list of valid names rather than silently registering
  nothing. Concretely:
  - `cmd/gen-cdp-tools/template.go` — `RenderRegister` now emits two new
    artifacts alongside `RegisterGenerated`: a sorted `Domains []string`
    catalog and `RegisterGeneratedFiltered(r, allowed map[string]bool)`
    that registers only the domains whose name is in `allowed`. Both go
    into `register_generated.go`, which means the drift test still
    enforces byte-equal regen output.
  - `internal/tools/cdptool/generated/register_generated.go` — regenerated
    to add `Domains` (18 names) and `RegisterGeneratedFiltered`.
  - `cmd/gen-cdp-tools/testdata/golden/register_generated.go` — golden
    fixture refreshed so the codegen golden test agrees with the new
    template output.
  - `internal/tools/cdptool/register.go` — new
    `RegisterFiltered(r, allowed []string)` that registers
    `cdp.selectTarget` (the cache primer is useful with or without the
    method tools) plus only the named domains. Empty allowed list still
    registers `selectTarget`. Also exposes `Domains() []string` so
    `cmd/office-addin-mcp` can validate user input without importing
    `internal/tools/cdptool/generated` directly.
  - `internal/mcp/registry.go` — `DefaultRegistry` signature changed from
    `(exposeRawCDP bool)` to `(sel CDPSelection)`. New `CDPSelection`
    type carries `Enabled bool, Domains []string`. The zero value
    registers nothing (default behavior); `Enabled=true, Domains=nil`
    matches the legacy `--expose-raw-cdp` semantic; `Enabled=true,
    Domains=[…]` filters.
  - `internal/mcp/registry_test.go` — call sites updated to the new
    signature; added `TestCDPDomainsFilterRegistersOnlyNamedDomains`
    asserting `Domains: ["DOM", "Page"]` registers `cdp.dOM.*` /
    `cdp.page.*` / `cdp.selectTarget` while filtering out
    `cdp.animation.*` / `cdp.runtime.*`.
  - `cmd/office-addin-mcp/main.go` — new `--cdp-domains` flag (CSV) and
    `--list-cdp-domains` flag. New `buildCDPSelection(enabled, csv)`
    helper trims/validates the CSV against `cdptool.Domains()` — typos
    fail with `unknown domain(s) [Foo]. Available: Accessibility,
    Animation, …`. A non-empty `--cdp-domains` implicitly enables CDP
    exposure, so users don't have to pass both `--expose-raw-cdp` and
    `--cdp-domains`. `--list-cdp-domains` prints the catalog and exits
    so users can discover the names without grepping the source.
  - `cmd/office-addin-mcp/main_test.go` — three new
    `TestBuildCDPSelection_*` cases cover empty CSV, the
    "non-empty domains imply enabled" rule, and the unknown-domain
    rejection path.

  Description-enrichment of the high-level `excel.*` / `page.*` /
  `addin.*` tools (concrete examples in their input schemas) and the
  `recommended_domains` manifest annotation are deferred to a follow-up
  — F7's structural piece (`--cdp-domains`) ships now; the doc-quality
  piece can land independently without touching the registry shape
  again.

- **MCP-native structured results (Title, OutputSchema, Annotations,
  StructuredContent).** Our MCP adapter previously registered tools with
  Name + Description + InputSchema only and emitted every result as
  `TextContent` (or `ImageContent` for screenshots). The current spec
  (https://modelcontextprotocol.io/specification/2025-11-25/server/tools)
  supports `title`, `outputSchema`, `annotations` (readOnly/destructive/
  idempotent/openWorld hints) and `structuredContent` for typed return
  payloads — all of which let MCP clients display tools more usefully and
  let LLMs branch on machine-validated output. Reasoning: shipping the
  infrastructure now (a) gives the adapter a stable contract, (b) means
  every future tool can opt into structured output without churning the
  adapter again, and (c) lets us surface read-only / idempotent hints on
  the safe tools so clients can skip confirmation prompts on probes like
  `addin.status`. Concretely:
  - `internal/tools/registry.go` — extended `Tool` with `Title string`,
    `OutputSchema json.RawMessage`, and `Annotations *Annotations`. New
    sibling `Annotations` type mirrors the SDK's `ToolAnnotations`
    (Title, ReadOnlyHint, DestructiveHint *bool, IdempotentHint,
    OpenWorldHint *bool) — pointer-bool fields keep the spec's "default
    true" semantics so leaving them nil inherits the default. Added a
    `BoolPtr(v) *bool` one-liner so annotation sites stay terse.
  - `internal/mcp/adapter.go` — `registerTool` copies Title,
    OutputSchema, and Annotations onto `sdk.Tool` (forwarding the
    annotation pointer fields verbatim). `makeHandler` now closes over
    the full `*tools.Tool` so it can pass a `hasOutputSchema bool` into
    `envelopeToResult`. `envelopeToResult` populates
    `res.StructuredContent = env.Data` whenever that flag is set, while
    still emitting the JSON-encoded `TextContent` block for clients that
    don't read structured output (per the spec recommendation that
    servers emit both for backwards compat).
  - `internal/tools/addintool/status.go` — added a complete
    `OutputSchema` (the JSON-Schema description of `statusOutput`) so
    `addin.status` is the demonstration of typed return data. Also gets
    `Title: "Add-in Status"`, `ReadOnlyHint: true`, `IdempotentHint: true`,
    `DestructiveHint: false`.
  - `internal/tools/addintool/{listtargets,contextinfo,ensurerunning}.go`,
    `internal/tools/lifecycletool/{detect,launch,stop}.go` — populated
    `Title` and `Annotations` for the lifecycle/probe surface. Probes
    (`addin.detect`, `addin.listTargets`, `addin.contextInfo`) get
    `ReadOnlyHint: true` so MCP clients can auto-allow them; lifecycle
    tools (`addin.launch`, `addin.ensureRunning`, `addin.stop`) get
    `IdempotentHint: true` and leave `DestructiveHint` at the spec
    default of true (or explicit true on `addin.stop`) so clients can
    prompt before re-firing them.
  - `internal/mcp/adapter_test.go` — new
    `TestEnvelopeToResultEmitsStructuredContent` exercises the
    structured-vs-text branch directly; new
    `TestListToolsExposesAnnotationsAndOutputSchema` round-trips a tool
    with all the new fields through SDK `tools/list` and asserts the
    client sees `Title`, `OutputSchema`, and `Annotations.ReadOnlyHint`.

  Codegen for the ~411 `cdp.*` tools (`cmd/gen-cdp-tools/template.go`
  enrichment with annotations + per-method outputSchema) is deferred to
  a follow-up — those tools still register with the now-baseline
  Name/Description/Schema shape, but the rest of the surface uses the
  new fields.

- **Windows WebView2 endpoint scan + `addin.status` aggregator.** The
  Windows discovery scan was a stub (`scan_windows.go` returning
  `ErrNotFound`), so a user who launched Excel without passing
  `--browser-url` would always fall through to the default :9222 probe and
  silently fail when Excel was on a non-default port. There was also no
  one-shot way to ask the bridge "is everything ready?" — agents had to
  chain `addin.detect` + `addin.listTargets` + `addin.contextInfo` and
  diff the results. Reasoning: a single status call with structured
  recoveryHints[] eliminates a multi-tool dance, and the scan turns a
  "manual port flag" gotcha into an automatic discovery on Windows.
  Concretely:
  - `internal/webview2/scan_windows.go` — replaced the stub with a real
    WebView2 scan. Excel inherits `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`
    as an env var, but the child `msedgewebview2.exe` processes parse it
    onto their command lines, so the scan shells out to
    `wmic process where "name='msedgewebview2.exe'" get CommandLine
    /format:list`, regex-extracts every `--remote-debugging-port=N`,
    deduplicates, and probes `http://127.0.0.1:N/json/version` via the
    existing `cdp.ResolveBrowserWSURL`. Returns the first responding
    endpoint or `ErrNotFound` if `wmic` is missing / nothing's running.
    The 5-second timeout on the wmic invocation prevents a slow box from
    blocking the discovery ladder for long.
  - `internal/webview2/scan_windows_test.go` *(new, build-tagged)* —
    tests the parser against representative `wmic /format:list` output:
    one process with two distinct ports, dedup of repeats, and an
    out-of-range / missing-flag scrub.
  - `internal/tools/addintool/status.go` *(new)* — `addin.status` tool
    (NoSession). Probes the configured endpoint via `webview2.Discover`,
    dials and calls `Target.getTargets`, classifies via
    `addin.ClassifyTargets`, and returns
    `{endpoint{source,browserUrl,wsUrl,reachable,error}, manifest{loaded,
    id,displayName,path,hosts}, targets[], recoveryHints[]}`. Always
    returns `tools.OK` — failures are encoded inside the payload so the
    agent can read both the reachability state and the recovery hint in
    one call instead of branching on `envelope.error`. RecoveryHints
    cover: unreachable endpoint → call `addin.ensureRunning`; missing
    manifest → call `addin.detect`; empty target list → taskpane may not
    be open yet.
  - `internal/tools/runtime.go` — added `RunEnv.Endpoint webview2.Config`.
    NoSession tools previously had no way to read the configured
    endpoint, which mattered because `addin.status` is the
    "is the endpoint reachable?" tool and needs to probe whatever the
    user/server resolved.
  - `internal/tools/dispatcher.go` — populates `env.Endpoint = req.Endpoint`
    on both the NoSession and pooled-session paths.
  - `internal/tools/addintool/register.go` — registers `Status()`.
  - `internal/tools/addintool/addintool_test.go` — extended the registry
    smoke test to assert `addin.ensureRunning` and `addin.status` show up;
    new `TestStatus_UnreachableEndpoint` runs the tool against
    `http://127.0.0.1:1` and asserts `reachable=false`,
    a non-empty `Endpoint.Error`, and a recoveryHint that mentions
    `addin.ensureRunning`.

- **`addin.ensureRunning` tool + `--launch-excel` startup flag.** Bringing
  Excel up with `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=9222"`
  and then sideloading the manifest by hand was the single biggest first-run
  friction point — agents that hit a closed Excel had to chain `addin.detect` +
  `addin.launch` and figure out from text which step was needed. Auto-launch
  is now opt-in via either entry point. Reasoning: probe-first means we never
  spawn Excel when one is already reachable, so the new behavior is safe to
  enable by default in a script while still respecting an explicit
  `--browser-url` / `--ws-endpoint`. Concretely:
  - `internal/launch/launcher.go` — new `LaunchIfNeeded(ctx, project, opts)`
    helper that probes `http://localhost:<port>/json/version` first (using
    the existing `ProbeCDPEndpoint`) and only delegates to `LaunchExcel`
    when the probe fails. Returns a `(result, source, err)` triple where
    `source` is `"preexisting"` or `"launched"` so callers can surface the
    distinction without needing to know which path ran.
  - `internal/tools/addintool/ensurerunning.go` *(new)* — `addin.ensureRunning`
    tool. Probes the configured port; if reachable, returns
    `{source: "preexisting", cdpUrl, manifestPath}` without spawning. If not
    reachable and the project was detected from `cwd`, runs the same
    `office-addin-debugging` path as `addin.launch`, then calls
    `RunEnv.SetEndpoint`/`SetManifest` so subsequent tool calls route to the
    new Excel. When detection fails, returns `addin_not_found` with a
    `RecoveryHint` pointing at `addin.detect` and
    `Details.recoverableViaTool: "addin.detect"`. `LaunchError`-wrapped
    failures get per-reason `RecoveryHint`s
    (`unsupported-platform` → "WebView2 sideloading is Windows-only…",
    `launcher-missing` → "install office-addin-debugging…",
    `port-already-configured` → "unset WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS…",
    `cdp-not-ready` / `dev-server-not-ready` → "retry with longer timeout").
  - `internal/tools/addintool/register.go` — registers `EnsureRunning()`.
  - `cmd/office-addin-mcp/main.go` — new `--launch-excel` flag plus an
    `autoLaunchExcel` helper. When set AND neither `--browser-url` nor
    `--ws-endpoint` was supplied, the binary detects the project from
    process cwd and calls `LaunchIfNeeded` before starting the MCP server,
    threading the resulting `cdpUrl` into `webview2.Config.BrowserURL`.
    Failures are logged via `slog.Warn` rather than fatal — the server
    still starts so the agent can call `addin.ensureRunning` interactively
    or fall back to manual launch.
  - `internal/launch/launcher_test.go` *(new)* — covers the
    "preexisting endpoint" path (httptest stub on a real local port) and
    the "no probe + nil project" guard rail.
  - `CLAUDE.md` — replaced the "Auto-launch is not implemented; do not add
    it without the user asking" note with an entry describing the opt-in
    `--launch-excel` flag and `addin.ensureRunning` tool, with the
    instruction not to auto-launch unconditionally and to honor explicit
    endpoint flags.

### Changed

- **Actionable error envelopes (recoveryHint + standard Details keys).**
  `EnvelopeError` previously surfaced a one-line `message` plus a free-form
  `details` map, which forced agents to parse English to figure out how to
  recover. Reasoning: an AI client that hits
  `code: "session_acquire_failed"` with no further context cannot tell
  whether to retry, call `addin.launch`, or back off — the dispatcher knew
  the answer locally but threw it away. Concretely:
  - `internal/tools/result.go` — added
    `EnvelopeError.RecoveryHint` (one-sentence English suggestion, omitted
    when empty) and documented six standard `Details` keys (`probedEndpoint`,
    `recoverableViaTool`, `cdpError`, `lastKnownTargets`, `manifestUrl`,
    `expectedUrlPattern`) that tools should populate whenever the data is
    locally available. Bumped `EnvelopeVersion` from `v0.3` to `v0.4`.
  - `internal/session/session.go` — exported sentinel errors
    `ErrReconnectBudgetExhausted` and `ErrDialFailed` so callers can branch
    with `errors.Is` instead of substring-matching the wrapped message. The
    dial-failure wrap uses `fmt.Errorf("%w: %w", ErrDialFailed, err)` so
    `errors.Is(ctx.DeadlineExceeded)` still fires when the dial timed out.
  - `internal/tools/dispatcher.go` — the single
    `session_acquire_failed` branch is replaced by a `classifyAcquireErr`
    helper that returns one of four codes
    (`session_reconnect_budget_exhausted`, `session_acquire_timeout`,
    `session_dial_failed`, `session_acquire_failed`), each with a
    code-specific `RecoveryHint` plus `Details["probedEndpoint"]` and (for
    the actionable cases) `Details["recoverableViaTool"] = "addin.launch"`.
  - `internal/tools/runtime.go` — `ClassifyCDPErr` now uses
    `errors.As` to pull the structured `*cdp.RemoteError` out of the chain,
    surfacing `{code, message, data}` as `Details["cdpError"]` so an agent
    can branch on the CDP-level code instead of regexing the message.
  - `internal/tools/addintool/errors.go` — `mapPayloadError` populates
    `RecoveryHint` for the well-known Office.js codes thrown by
    `internal/js/_preamble.js`: `office_unavailable` / `excel_unavailable`,
    `office_ready_failed` / `office_ready_timeout`, and
    `requirement_unmet` / `requirement_check_failed`.
  - `internal/session/session_test.go` — the existing reconnect-budget
    test now asserts via `errors.Is(err, ErrDialFailed)` and
    `errors.Is(err, ErrReconnectBudgetExhausted)` rather than substring
    matching, validating the sentinel-error contract.
  - `internal/tools/dispatcher_test.go` — new
    `TestEnvelopeErrorRecoveryHints` table-driven test covers the four
    acquire-failure modes and asserts code, category, `probedEndpoint`,
    `recoverableViaTool`, and recoveryHint substring.
  - `internal/tools/testdata/golden/{success,validation_error,cdp_error,timeout,unknown_tool}.json`
    bumped to `v0.4`.

- **Structured logging + per-call request correlation.** The binary
  now configures `log/slog` with a JSON handler at startup. The
  dispatcher (`internal/tools/dispatcher.go`) generates a
  cryptographically-random 16-hex-char request id at the top of every
  `Dispatch`, threads it through the call's `context.Context` via the
  new `internal/log` helper, copies it into the envelope's
  `diagnostics.requestId`, and emits `dispatch.start`/`dispatch.end`
  debug log lines tagged with it. Downstream layers can pick the id
  off ctx — `internal/cdp/connection.go` `Send` already does, so each
  CDP round-trip can be tied back to one tool call without having to
  reverse-engineer the call from a wall-clock window. Reasoning: the
  server was previously silent unless `--log-file` was set, and even
  then it wrote unstructured `fmt.Fprintf` lines with no correlation
  id, which made it impossible to tell which CDP send belonged to
  which tool call when an MCP client issued anything in parallel.
  Concretely:
  - `internal/log/log.go` *(new)* — leaf package (stdlib only) with
    `WithRequestID`/`RequestID` for ctx-scoped ids and a
    `RecoverGoroutine(name)` defer-friendly panic catcher that logs at
    `ERROR` level via slog.
  - `internal/tools/result.go` — added `Diagnostics.RequestID`. Bumped
    `EnvelopeVersion` from `v0.2` to `v0.3` (per-call hex correlation
    id stamped by the dispatcher and threaded through ctx).
  - `internal/tools/testdata/golden/{success,validation_error,cdp_error,timeout,unknown_tool}.json`
    bumped to `v0.3`. `canonicalize` in
    `internal/tools/dispatcher_test.go` now zeroes `RequestID` so the
    randomized id does not break golden diffs.
  - `cmd/office-addin-mcp/main.go` — initializes `slog.SetDefault`
    with `slog.NewJSONHandler` writing to `--log-file` (or stderr) at
    the level chosen by the new `--log-level` flag (`debug|info|warn|
    error`, default `info`). The mcp-server-exit error message now
    goes through `slog.Error` rather than a raw `fmt.Fprintf`.
  - `internal/tools/dispatcher_test.go` — new `TestDispatchStampsRequestID`
    asserts `Diagnostics.RequestID` is 16 hex chars and unique across
    five back-to-back calls.

- **Panic recovery on every long-lived goroutine.** The CDP
  `readLoop` and the launch package's `drainPipe`/`waitChild`
  goroutines plus the session `Manager.gcLoop` previously had no
  `defer recover()`, so a malformed frame, unexpected EOF in a child
  pipe, or a stray nil deref would silently kill the goroutine and
  leave the surrounding subsystem in an undefined state. Reasoning:
  for a production AI bridge, "the server stopped responding" with no
  log line is the worst possible failure mode — recover() costs
  essentially nothing and turns the panic into a single structured
  log entry plus a clean shutdown of the affected resource.
  Concretely:
  - `internal/cdp/connection.go` `readLoop` — `defer` block now
    catches any panic, logs it via `slog.Error` with the goroutine
    name and panic value, and calls `closeWithErr` so pending
    requesters get `ErrClosed` instead of hanging forever. Also added
    a `slog.Debug("cdp.send", ...)` tagged with the request id (when
    one is on ctx) so per-CDP-call logs correlate with the dispatcher's
    `dispatch.start`/`dispatch.end` lines.
  - `internal/session/manager.go` `gcLoop` — wraps the loop body in
    `defer log.RecoverGoroutine("session.gcLoop")`.
  - `internal/launch/devserver.go` `drainPipe` and the anonymous
    `cmd.Wait()` goroutine inside `waitChild` — same wrap.

- **Documented stdio-only contract.** `cmd/office-addin-mcp/main.go` already
  rejects positional arguments and the historic `call` / `daemon` /
  `serve --stdio` subcommands have been gone since the MCP-over-stdio
  rewrite, but the README continued to advertise them and the version
  metadata across `mcp.json`, the npm package files, and the README
  banner had drifted. Reasoning: misleading docs cause new users to
  type a command and immediately hit `unexpected argument`, eroding
  trust on first contact, and the version disagreement makes it
  impossible to tell at a glance which tag a checkout corresponds to.
  Concretely:
  - `README.md` now describes the binary as MCP-over-stdio only;
    removed the "Daemon mode" and "Stdio mode (MCP protocol)" sections,
    the broken `office-addin-mcp call …` Quick-start examples, the
    `--no-daemon` / `--idle-timeout` flag rows, and the Subcommands
    table. Added the missing `--ws-endpoint`, `--log-file`, and
    `--version` flags. Replaced the hard-coded `v0.1.0` banner with a
    pointer to the GitHub Releases page so the doc no longer goes
    stale on every tag.
  - `npm/main/package.json`, `npm/win32-x64/package.json`,
    `npm/darwin-x64/package.json`, `npm/darwin-arm64/package.json`,
    `npm/linux-x64/package.json`, `npm/linux-arm64/package.json` —
    versions (and `optionalDependencies` pins in `npm/main`) bumped
    from `0.1.0`/`0.1.1` to `0.2.0` to match `mcp.json`. The release
    workflow re-stamps these from the tag at publish time, so the
    checked-in values are informational, but consistent baseline state
    keeps `git diff` against a release tag readable.

### Added

- **Release workflow drift check.**
  `.github/workflows/release.yml` now runs a "Contract drift check"
  step before the existing version comparison. It (a) greps `README.md`
  for the four removed subcommand invocations
  (`office-addin-mcp call`, `office-addin-mcp daemon`,
  `office-addin-mcp serve --stdio`, `office-addin-mcp list-tools`)
  and (b) asserts every checked-in
  `npm/<platform>/package.json` and `npm/main/package.json` version
  agrees with `mcp.json`. Reasoning: the prior behavior allowed a tag
  to ship even when the README still advertised dead subcommands or
  one of the npm packages had been hand-edited but its peers missed
  — this check fails fast at the start of the release pipeline so the
  drift is caught before publishing to npm or the MCP registry.
- **`page.consoleLog`, `page.networkLog`, `page.networkBody`.** Three
  event-buffer-backed inspection tools for the high-level surface. The
  first call against a target subscribes to the relevant CDP events
  (`Runtime.consoleAPICalled` / `Runtime.exceptionThrown` /
  `Log.entryAdded` for console; `Network.*` for network) and starts a
  pump goroutine that drains them into a per-target ring buffer kept on
  the session. Subsequent calls drain accumulated entries via a
  monotonic `seq` cursor (`sinceSeq` → `lastSeq`). Bounded at 1000
  entries by default, overridable via `maxBuffer`. Buffers are
  per-`cdpSessionId`, so flipping pages with `pages.select` preserves
  prior-target output; everything is cleared on CDP reconnect.
  `page.networkLog` correlates `requestWillBeSent` /
  `responseReceived` / `loadingFinished` / `loadingFailed` into one
  record per completed request, with optional `includeHeaders` and
  status / URL / failed-only filters. `page.networkBody` fetches the
  response body for a logged `requestId`, capped at 5 MiB.
- **`cdp.Connection.SubscribeMethods`.** Multi-method subscribe form
  used by the network pump to preserve cross-method ordering
  (`requestWillBeSent` before `responseReceived` for the same
  `requestId`) — single-method subscribe channels couldn't guarantee
  this under a multi-channel `select`.
- **Phase 6: raw CDP gated behind `--expose-raw-cdp`.** The default
  `tools/list` advertises only the high-level Office add-in surface
  (`addin.*`, `pages.*`, `page.*`, `excel.*`, plus the interaction tools
  registered as `page.click` / `page.fill` / `page.hover` /
  `page.typeText` / `page.pressKey`). Pass `--expose-raw-cdp` (or set
  `OFFICE_ADDIN_MCP_EXPOSE_RAW_CDP=1`) to also register the ~411
  code-generated `cdp.*` tools and the `cdp.selectTarget` cache primer.
- **Phase 5: Excel tool surface expansion (11 → 37).** New `excel.*` tools
  cover workbook (`workbookInfo`, `calculationState`, `listNamedItems`,
  `customXmlParts`, `settingsGet`), worksheet (`worksheetInfo`,
  `listComments`, `listShapes`), range (`activeRange`, `usedRange`,
  `rangeProperties`, `rangeFormulas`, `rangeSpecialCells`, `findInRange`,
  `listConditionalFormats`, `listDataValidations`), tables (`listTables`,
  `tableInfo`, `tableRows`, `tableFilters`), charts (`listCharts`,
  `chartInfo`, `chartImage`), and PivotTables (`listPivotTables`,
  `pivotTableInfo`, `pivotTableValues`). Read tools that materialize 2D
  grids cap output at 1000 cells and flag truncation. `excel.chartImage`
  payload returns `{mimeType, data}`, surfaced as an MCP `ImageContent`
  block by the adapter.
- **Code-generated CDP tool surface (~411 tools across 18 domains).**
  Every method named in `CdpProtocols.md` and present in the vendored
  `browser_protocol.json` / `js_protocol.json` is now exposed as
  `cdp.<lowerDomain>.<lowerMethod>`. See
  [docs/cdp-tools.md](docs/cdp-tools.md) for the index.
- `cmd/gen-cdp-tools` — code generator driven by `cdp/manifest.yaml`
  (policy overlay) plus the vendored protocol JSON. Deterministic
  output (sorted iteration + in-process `go/format`); drift test
  enforces no-diff against checked-in output.
- `scripts/build_manifest.py` — regenerates the manifest from
  `CdpProtocols.md` with hand-curated DANGEROUS / BINARY_FIELD /
  BROWSER_SCOPED / AUTO_ENABLE tables.
- Vendored Chrome devtools-protocol JSON at SHA
  `470fb6a42cbcaf446b516d8fc7738f9723cba5fc` (r1621552, 2026-04-28),
  pinned in `cdp/protocol/VERSION`.
- Lazy domain enabling — `Session.EnsureEnabled` issues
  `<Domain>.enable` exactly once per `(cdpSessionID, domain)` pair;
  cleared on reconnect (`dropConnLocked`). `RunEnv.EnsureEnabled` is
  the dispatcher-side hook generated tools call before their first
  command.
- `--allow-dangerous-cdp` flag (and `OAMCP_ALLOW_DANGEROUS_CDP=1`)
  on `call` / `serve` / `daemon`. Generated dangerous tools
  (`cdp.browser.crash`, `cdp.runtime.terminateExecution`, etc.)
  refuse with `category=unsupported, code=dangerous_disabled` unless
  set. Process-wide on the daemon; no per-call override.
- Binary `outputPath` codegen for `cdp.page.captureScreenshot`,
  `cdp.page.printToPDF`, `cdp.page.captureSnapshot`. Setting the
  param decodes the base64 result to disk and returns
  `{path, sizeBytes, mimeType}` instead of raw bytes.
  `tools.WriteBinaryFieldOutput` is the shared helper.

### Changed

- `cdp.evaluate`, `cdp.getTargets`, and `browser.navigate` are
  **removed**. Use `page.evaluate`, `pages.list`, and `page.navigate`
  instead. Power users who still want raw CDP access can run with
  `--expose-raw-cdp` and call `cdp.runtime.evaluate`,
  `cdp.target.getTargets`, and `cdp.page.navigate` directly.
  `cdp.selectTarget` remains hand-written (primes the per-session
  selector cache; no direct CDP equivalent) and is now also gated
  behind `--expose-raw-cdp`.
- `.golangci.yml` migrated from v1 to v2 syntax with `errcheck`
  exclusions for `fmt.Fprint*` and `(io.Closer).Close` (idiomatic
  no-recovery patterns). One pre-existing gofumpt fix in
  `internal/cdp/runtime_test.go` (collapsed two `var _ = …` into a
  `var (…)` block).

### Tests

- Generator: `TestGolden` + `TestDeterministic` (fixture-based);
  `TestLiveManifestDrift` (replaces `go generate && git diff`).
- Naming: `TestGeneratedToolNamesMatchPattern` enforces
  `^cdp\.[a-z][a-zA-Z]*\.[a-z][a-zA-Z]*$` on every generated tool.
- Lazy enable: `TestEnsureEnabledOnceAcrossCalls`,
  `TestEnsureEnabledPerCDPSession`,
  `TestEnsureEnabledClearedOnReconnect`.
- P6 codegen paths: `TestDangerousRefusedWithoutFlag`,
  `TestBinaryOutputPathWritesFile`,
  `TestBinaryOutputPathOmittedReturnsRaw`.

### Notes

- Refreshing the vendored protocol JSON is a breaking change at the
  result-shape boundary — generated tools pass Chrome's response
  through verbatim. See
  [docs/migration-notes.md](docs/migration-notes.md#refreshing-the-protocol)
  for the procedure.

## v0.1.0 — 2026-04-29

First tagged release. Implements PLAN.md Phases 0 through 6.

### Added

- Hand-rolled CDP client over `gorilla/websocket` with id-correlated
  request/response and method-keyed event subscribe
  (`internal/cdp/connection.go`).
- WebView2 endpoint discovery with priority ladder: explicit
  ws-endpoint > explicit browser-url > default `http://127.0.0.1:9222`
  > Windows scan stub (`internal/webview2/`).
- Tool registry with JSON-Schema-validated boundary
  (`santhosh-tekuri/jsonschema/v5`) and uniform `{ok, data, error,
  diagnostics}` envelope versioned via `EnvelopeVersion`
  (`internal/tools/`).
- 15 tools across three domains:
  - `cdp.evaluate`, `cdp.getTargets`, `cdp.selectTarget`
  - `browser.navigate`
  - `excel.readRange`, `excel.writeRange`, `excel.listWorksheets`,
    `excel.getActiveWorksheet`, `excel.activateWorksheet`,
    `excel.createWorksheet`, `excel.deleteWorksheet`,
    `excel.getSelectedRange`, `excel.setSelectedRange`,
    `excel.createTable`, `excel.runScript`
- Office.js execution stack: 11 embedded payloads + a preamble
  (`__officeError`, `__ensureOffice`, `__requireSet`, `__runExcel`)
  with structured error reporting (`internal/officejs/`,
  `internal/js/`).
- Persistent session pool with sticky selector cache, sliding-window
  reconnect budget (3 in 60s), and idle GC (`internal/session/`).
  Session reuse drops `cdpRoundTrips` from ~3 to 1 after the first
  call.
- Daemon mode: HTTP/1.1 server on `127.0.0.1` with bearer-token auth,
  endpoints `/v1/{health,call,list-tools,status}`, socket file at
  `os.UserCacheDir()/office-addin-mcp/daemon.json` mode 0600
  (`internal/daemon/`).
- `serve --stdio` subcommand for stdio-mode hosts.
- `call` subcommand auto-routes to a healthy daemon when the socket
  file is present; `--no-daemon` forces in-process dispatch.
- `list-tools` subcommand emits the registered catalog with JSON
  Schemas.
- Documentation: README quick-start, `docs/architecture.md`,
  `docs/tool-contracts.md`, `docs/migration-notes.md`.

### Envelope

`v0.2` — `sessionId` is the user/Phase-5 session id; `cdpSessionId`
carries the CDP flatten session id; `cdpRoundTrips` diagnostic added.

### Tests

50+ unit and integration tests including:

- CDP message correlation, exception unwrap, ctx timeout, post-close
  failure, event dispatch.
- Endpoint discovery priority ladder.
- Tool registry duplicate / empty-name / nil-Run / bad-schema; JSON
  Schema validation (required, additionalProperties, null-as-empty,
  decode errors).
- Golden-JSON envelope shapes for success / validation_error /
  cdp_error / timeout / unknown_tool.
- Office.js executor: success unwrap, OfficeError unwrap with
  debugInfo, protocol exception, transport error pass-through, U+2028
  escape via `json.Marshal` default.
- Per-tool integration tests against a fakeBrowser (cdp.evaluate,
  cdp.getTargets filter, cdp.selectTarget, browser.navigate,
  excel.readRange success + Office.js error, excel.runScript args).
- Session: sticky cache hit/miss, reconnect-budget exhaustion,
  selection cache cleared on reconnect, idle GC, Drop closes
  connection.
- Daemon: health + auth (401 without bearer), Stop removes socket
  file, **10 sequential `call` invocations against the daemon use
  exactly 1 WS dial** with steady-state cdpRoundTrips=1.

### Known limitations

- No auto-launch of Excel via `office-addin-debugging` (PLAN.md §11
  Q7); launch Excel manually with `--remote-debugging-port=9222`.
- WebView2 user-data-dir scanning is stubbed (PLAN.md §10).
- `excel.runScript` ships the permissive variant accepting arbitrary
  async JS bodies (PLAN.md §11 Q5).
- No telemetry / token-counting (PLAN.md §11 Q6).
- Per-tool golden response JSON against a real fixture workbook is
  populated manually per the checklist in `docs/migration-notes.md`.
