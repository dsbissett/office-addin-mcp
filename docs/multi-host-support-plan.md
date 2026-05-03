# Plan: Multi-Host Office Add-in Support (Word, Outlook, PowerPoint, OneNote)

## Context

The server currently supports only Excel via `excel.*` tools backed by Office.js payloads and the `Excel.run()` API. The CDP/session/MCP infrastructure is entirely host-agnostic. These changes generalize the JS preamble, generalize manifest detection, extract the shared payload runner into a common package, and add a tool package + JS payloads for each new host (Word, Outlook, PowerPoint, OneNote).

---

## Dependency Order

```
F1 (officetool)
    ↓
F2 (preamble) ─────────────┐
F3 (detect)                │
    ↓                      ↓
F4 word tools          F5 outlook tools
F6 powerpoint tools    F7 onenote tools
    ↓ (all host packages)
F8 requirements sets
F9 --launch-addin flag
    ↓
Registry wiring (one commit)
```

F1 unblocks all 4 host packages (they import `officetool.RunPayload`).  
F2 and F3 are independent of each other; both can land alongside the host packages.

---

## Critical Files

| File | What changes |
|---|---|
| `internal/js/_preamble.js` | Remove Excel check from `__ensureOffice`; add `__runWord`, `__runPowerPoint`, `__runOneNote`, `__runOutlook` |
| `internal/launch/detect.go` | Broaden XML/JSON manifest detection from Workbook-only to any Office add-in |
| `internal/launch/detect_test.go` | Update `TestDetectAddin_NonWorkbookXMLRejected` to expect success, not failure |
| `internal/tools/exceltool/runner.go` | Delegate `runPayload` body to `officetool.RunPayload`; keep unexported wrapper |
| `internal/tools/officetool/runner.go` | **New**: exported `RunPayload`, `SelectorFields`, `TargetSelectorBase` |
| `internal/mcp/registry.go` | Add 4 new `Register()` calls |
| `internal/addin/requirements.go` | Extend `StandardRequirementSets` for Word/Mailbox/PowerPoint/OneNote |
| `internal/tools/addintool/errors.go` | Update `excel_unavailable` hint to be host-agnostic |
| `internal/tools/lifecycletool/detect.go` | Remove "Excel" from description strings |
| `cmd/office-addin-mcp/main.go` | Add `--launch-addin` flag; keep `--launch-excel` as deprecated alias |

New directories: `internal/tools/wordtool/`, `internal/tools/outlooktool/`, `internal/tools/powerpointtool/`, `internal/tools/onenotetool/`

New JS files in `internal/js/`: `word_*.js`, `outlook_*.js`, `powerpoint_*.js`, `onenote_*.js`

---

## F1 — Extract shared payload runner

Create `internal/tools/officetool/runner.go`. Export three things so every host package can import just this one file:

```go
package officetool

// TargetSelectorBase — JSON schema snippet embedded in each host tool's schema
const TargetSelectorBase = `
    "targetId":   {"type": "string", "description": "Exact target id; mutually exclusive with urlPattern."},
    "urlPattern": {"type": "string", "description": "Substring of the target URL (e.g. add-in taskpane URL)."}
`

// SelectorFields — embed in each host tool's params struct
type SelectorFields struct {
    TargetID   string `json:"targetId,omitempty"`
    URLPattern string `json:"urlPattern,omitempty"`
}

func (s SelectorFields) Selector() tools.TargetSelector { ... }

// RunPayload — identical body to exceltool.runPayload
func RunPayload(ctx context.Context, env *tools.RunEnv, sel tools.TargetSelector, payload string, args any) tools.Result { ... }
```

Then simplify `internal/tools/exceltool/runner.go`: change `runPayload` to forward to `officetool.RunPayload`. The private `selectorFields`/`targetSelectorBase` types stay in `exceltool` for now — host packages will use the exported `officetool` versions.

Verify: `go test ./...` passes unchanged.

---

## F2 — Generalize preamble

Edit `internal/js/_preamble.js`:

1. **`__ensureOffice`**: Remove the `globalThis.Excel` check and its `excel_unavailable` throw. Keep only the `globalThis.Office` check.

2. Keep `__runExcel(fn)` unchanged (Excel payloads keep calling it).

3. Add three new run helpers:
```js
async function __runWord(fn) {
  await __ensureOffice();
  try {
    return await Word.run(async (context) => fn(context));
  } catch (e) {
    if (e && e.__officeError) throw e;
    throw __officeError((e && e.code) || 'word_run_failed', (e && e.message) || String(e), { debugInfo: e && e.debugInfo });
  }
}

async function __runPowerPoint(fn) {
  await __ensureOffice();
  try {
    return await PowerPoint.run(async (context) => fn(context));
  } catch (e) {
    if (e && e.__officeError) throw e;
    throw __officeError((e && e.code) || 'powerpoint_run_failed', (e && e.message) || String(e), { debugInfo: e && e.debugInfo });
  }
}

async function __runOneNote(fn) {
  await __ensureOffice();
  try {
    return await OneNote.run(async (context) => fn(context));
  } catch (e) {
    if (e && e.__officeError) throw e;
    throw __officeError((e && e.code) || 'onenote_run_failed', (e && e.message) || String(e), { debugInfo: e && e.debugInfo });
  }
}

async function __runOutlook(fn) {
  await __ensureOffice();
  try {
    return await fn(Office.context.mailbox);
  } catch (e) {
    if (e && e.__officeError) throw e;
    throw __officeError((e && e.code) || 'outlook_run_failed', (e && e.message) || String(e), { debugInfo: e && e.debugInfo });
  }
}
```

Update `internal/tools/addintool/errors.go:44` — remove `excel_unavailable` from the switch case; the hint for `office_unavailable` alone is sufficient (now covers all hosts).

Update `internal/officejs/payloads_test.go` (`TestPreambleEmbedded`) — add `__runWord` to the expected string list.

Verify: `go test ./...` passes; existing Excel tools still work.

---

## F3 — Generalize manifest detection

Edit `internal/launch/detect.go`:

1. Rename `isWorkbookXMLManifest` → `isOfficeXMLManifest`: keep `reOfficeApp` check, **remove** `reHostName` check.
2. Rename `isWorkbookJSONManifest` → `isOfficeJSONManifest`: accept any non-empty `scopes` array (remove the `"workbook"` string equality check).
3. Update `detectManifest` to call the renamed functions.
4. Update `ErrNoProject` message: `"launch: no Office add-in project detected"`.
5. Update `Project` struct doc comment: remove "Excel".

Edit `internal/launch/detect_test.go`:

- Update existing test fixture strings (they use `Host Name="Workbook"` — these still work since we just accept any XML with `<OfficeApp`).
- Rename `TestDetectAddin_NonWorkbookXMLRejected` → `TestDetectAddin_NonWorkbookXMLAccepted` and flip the assertion to expect **success** (a `Host Name="Document"` manifest is now valid).

Edit `internal/tools/lifecycletool/detect.go` — update the `Detect()` tool description to say "Office add-in project" instead of "Office Excel add-in project"; remove "workbook-scoped manifest" wording.

Verify: `go test ./...` passes.

---

## F4 — Word tools

**JS payloads** in `internal/js/` — all use `__runWord(async ctx => { ... return { result: ... }; })`:

| File | Tool name | What it does |
|---|---|---|
| `word_read_body.js` | `word.readBody` | `context.document.body.load("text")` → `{text}` |
| `word_write_body.js` | `word.writeBody` | `body.insertText(args.text, args.location)` where location: `"replace"\|"start"\|"end"` |
| `word_read_paragraphs.js` | `word.readParagraphs` | `body.paragraphs.load("items/text,items/style")` → `[{text, style}]` |
| `word_insert_paragraph.js` | `word.insertParagraph` | `body.insertParagraph(args.text, args.location)`, loads style → `{style}` |
| `word_read_selection.js` | `word.readSelection` | `context.document.getSelection().load("text")` → `{text}` |
| `word_search_text.js` | `word.searchText` | `body.search(args.query, {matchCase, matchWholeWord}).load("text")` → `[{text}]` |
| `word_read_properties.js` | `word.readProperties` | `context.document.properties.load(...)` → `{title, author, ...}` |
| `word_run_script.js` | `word.runScript` | Escape hatch — `Word.run(async ctx => eval(args.script)(ctx))` |

**Go package** `internal/tools/wordtool/`:
- `runner.go` — package-level `runPayload` forwarding to `officetool.RunPayload`; `selectorFields` / `emptySelectorParams` types using `officetool.SelectorFields` and `officetool.TargetSelectorBase`
- `document.go` — tool constructors for the 8 tools
- `register.go` — `Register(r *tools.Registry)` wiring all 8

Pattern to follow: exactly mirrors `exceltool/workbook.go` + `exceltool/script.go` with `word.*` names and `__runWord` payloads.

---

## F5 — Outlook tools

**JS payloads** — all use `__runOutlook(async mailbox => { ... })`:

| File | Tool name | What it does |
|---|---|---|
| `outlook_read_item.js` | `outlook.readItem` | `mailbox.item` properties: `{subject, itemType, itemClass, conversationId, dateTimeCreated}` |
| `outlook_get_body.js` | `outlook.getBody` | Promisifies `item.body.getAsync(coercionType, cb)` → `{body, coercionType}` |
| `outlook_set_body.js` | `outlook.setBody` | Promisifies `item.body.setAsync(content, {coercionType}, cb)` → `{ok}` |
| `outlook_get_subject.js` | `outlook.getSubject` | Promisifies `item.subject.getAsync(cb)` (compose) or reads `item.subject` (read) → `{subject}` |
| `outlook_set_subject.js` | `outlook.setSubject` | Promisifies `item.subject.setAsync(args.subject, cb)` → `{ok}` |
| `outlook_get_recipients.js` | `outlook.getRecipients` | Promisifies `item.to.getAsync` and `item.cc.getAsync` → `{to:[...], cc:[...]}` |
| `outlook_run_script.js` | `outlook.runScript` | Escape hatch — `fn(mailbox)` where `fn = eval(args.script)` |

Note: Outlook's callback APIs (`getAsync`, `setAsync`) must be wrapped in `new Promise((resolve, reject) => item.body.getAsync(..., r => r.status==='succeeded' ? resolve(r.value) : reject(...)))` inside each payload.

**Go package** `internal/tools/outlooktool/` — same structure as wordtool, 7 tools.

---

## F6 — PowerPoint tools

**JS payloads** — all use `__runPowerPoint(async ctx => { ... })`:

| File | Tool name | What it does |
|---|---|---|
| `powerpoint_read_presentation.js` | `powerpoint.readPresentation` | `context.presentation.load("title")`, `slides.load("items/id")` → `{title, slideCount}` |
| `powerpoint_read_slides.js` | `powerpoint.readSlides` | `slides.load("items/id,items/shapes/items/name")` → `[{id, shapeNames}]` |
| `powerpoint_read_slide.js` | `powerpoint.readSlide` | Shapes on slide at `args.slideIndex` → `[{name, shapeType, left, top, width, height}]` |
| `powerpoint_add_slide.js` | `powerpoint.addSlide` | `context.presentation.slides.add()` → `{id}` |
| `powerpoint_read_selection.js` | `powerpoint.readSelection` | `context.presentation.getSelectedSlides().load("items/id")` → `[{id}]` |
| `powerpoint_run_script.js` | `powerpoint.runScript` | Escape hatch via `PowerPoint.run` |

**Go package** `internal/tools/powerpointtool/` — same structure, 6 tools.

---

## F7 — OneNote tools

**JS payloads** — all use `__runOneNote(async ctx => { ... })`:

| File | Tool name | What it does |
|---|---|---|
| `onenote_read_notebooks.js` | `onenote.readNotebooks` | `context.application.notebooks.load("items/name,items/id")` → `[{id, name}]` |
| `onenote_read_sections.js` | `onenote.readSections` | `getActiveNotebook().sections.load(...)` → `[{id, name}]` |
| `onenote_read_pages.js` | `onenote.readPages` | `getActiveSection().pages.load(...)` → `[{id, title}]` |
| `onenote_read_page.js` | `onenote.readPage` | `getActivePage().load("title,contents/items/type,contents/items/id")` → `{title, contents}` |
| `onenote_add_page.js` | `onenote.addPage` | `getActiveSection().addPage(args.title)` → `{id, title}` |
| `onenote_run_script.js` | `onenote.runScript` | Escape hatch via `OneNote.run` |

**Go package** `internal/tools/onenotetool/` — same structure, 6 tools.

---

## F8 — Extend requirement sets

Edit `internal/addin/requirements.go` — append to `StandardRequirementSets`. Note: the field is `MinVersion` (not `Version`):

```go
// Word
{Name: "WordApi", MinVersion: "1.1"},
{Name: "WordApi", MinVersion: "1.2"},
{Name: "WordApi", MinVersion: "1.3"},
{Name: "WordApi", MinVersion: "1.4"},
// Outlook
{Name: "Mailbox", MinVersion: "1.1"},
{Name: "Mailbox", MinVersion: "1.5"},
{Name: "Mailbox", MinVersion: "1.8"},
{Name: "Mailbox", MinVersion: "1.10"},
{Name: "Mailbox", MinVersion: "1.13"},
// PowerPoint
{Name: "PowerPointApi", MinVersion: "1.1"},
{Name: "PowerPointApi", MinVersion: "1.2"},
{Name: "PowerPointApi", MinVersion: "1.3"},
// OneNote
{Name: "OneNoteApi", MinVersion: "1.1"},
```

---

## F9 — Add `--launch-addin` flag

Edit `cmd/office-addin-mcp/main.go`:
- Add `--launch-addin` flag with the same behavior as `--launch-excel` (both call `autoLaunchExcel` — which already works generically since it just calls `launch.DetectAddin`).
- Keep `--launch-excel`; update its description to `"Deprecated alias for --launch-addin."`.
- The `if *launchExcel` block becomes `if *launchExcel || *launchAddin`.
- Add `--launch-addin` to `writeUsage`.

---

## Registry wiring

Edit `internal/mcp/registry.go` — add four imports and four `Register` calls after `exceltool.Register(r)`:

```go
wordtool.Register(r)
outlooktool.Register(r)
powerpointtool.Register(r)
onenotetool.Register(r)
```

---

## Verification

After each step:
1. `go test ./...` passes
2. `golangci-lint run` passes

End-to-end (manual):
- `office-addin-mcp --list-cdp-domains` exits cleanly (smoke test binary builds)
- `list-tools` MCP call confirms `word.*` (8), `outlook.*` (7), `powerpoint.*` (6), `onenote.*` (6) appear
- Confirm existing `excel.*` tools still work (preamble change must not regress Excel)
- For Word: point server at a Word add-in taskpane; call `word.readBody` — confirm non-error response

---

## Tool Count Summary

| Package | New tools |
|---|---|
| `wordtool` | 8 |
| `outlooktool` | 7 |
| `powerpointtool` | 6 |
| `onenotetool` | 6 |
| **Total new** | **27** |