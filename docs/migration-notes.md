# Migration notes — TypeScript `excel-webview2-mcp` → Go `office-addin-mcp`

This file is the working contract for "what does v1 ship?" Phase 6 finalizes
it; for now it tracks the parity matrix and the manual Excel checklist that
backs the Phase 4 acceptance criterion.

## Tool parity (Phase 1–4 surface)

| TS tool                       | Go tool                       | Status   | Notes                                                             |
| ----------------------------- | ----------------------------- | -------- | ----------------------------------------------------------------- |
| `cdp_evaluate`                | `cdp.evaluate`                | ported   | Adds `targetId` / `urlPattern` selectors; envelope-uniform         |
| `cdp_get_targets`             | `cdp.getTargets`              | ported   | `type` / `urlPattern` / `includeInternal` filters                  |
| `cdp_select_target`           | `cdp.selectTarget`            | ported   | Persistence of selection deferred to Phase 5 (sessions)            |
| `browser_navigate`            | `browser.navigate`            | ported   | Page.navigate; surfaces `errorText` as `category=protocol`         |
| `excel_read_range`            | `excel.readRange`             | ported   |                                                                    |
| `excel_write_range`           | `excel.writeRange`            | ported   | `anyOf` requires `values` or `formulas` or `numberFormat`          |
| `excel_list_worksheets`       | `excel.listWorksheets`        | ported   |                                                                    |
| `excel_get_active_worksheet`  | `excel.getActiveWorksheet`    | ported   |                                                                    |
| `excel_activate_worksheet`    | `excel.activateWorksheet`     | ported   | Requires ExcelApi 1.7                                              |
| `excel_create_worksheet`      | `excel.createWorksheet`       | ported   |                                                                    |
| `excel_delete_worksheet`      | `excel.deleteWorksheet`       | ported   |                                                                    |
| `excel_get_selected_range`    | `excel.getSelectedRange`      | ported   |                                                                    |
| `excel_set_selected_range`    | `excel.setSelectedRange`      | ported   |                                                                    |
| `excel_run_script`            | `excel.runScript`             | ported   | Permissive variant — see PLAN.md §11 Open Question 5               |
| `excel_create_table`          | `excel.createTable`           | ported   |                                                                    |
| `lighthouse_*`                | —                             | dropped  | Out of scope for v1                                                |
| `performance_*`               | —                             | dropped  | Out of scope                                                       |
| `screencast_*`                | —                             | dropped  | Out of scope                                                       |
| `take_memory_snapshot`        | —                             | dropped  | Out of scope                                                       |
| in-page DevTools interop      | —                             | dropped  | Out of scope                                                       |

## Renames

- `snake_case` → `<domain>.<verb>Noun>`. Example: `excel_read_range` → `excel.readRange`.
- `McpResponse` markdown → `{ok, data, error, diagnostics}` JSON envelope (versioned via `diagnostics.envelopeVersion`).

## Manual Excel checklist (Phase 4 acceptance)

These need a real Excel + a sample add-in attached to the same workbook used
by the TS repo's e2e tests. Run Excel with the WebView2 debug port enabled,
then exercise each tool through `office-addin-mcp call --tool ... --param ...`
and capture the resulting envelope into `testdata/golden/excel/<tool>.json`.

Workbook fixture: `Sheet1` populated with values in `A1:D10`, plus a second
sheet named `Hidden` set to `visibility: hidden`.

- [ ] `excel.listWorksheets` — returns `Sheet1` and `Hidden`; `Hidden.visibility = "Hidden"`.
- [ ] `excel.getActiveWorksheet` — `name = "Sheet1"`.
- [ ] `excel.activateWorksheet` `{name: "Hidden"}` — succeeds; subsequent `getActiveWorksheet` returns `"Hidden"`.
- [ ] `excel.createWorksheet` `{name: "Tmp"}` — creates and returns the new sheet.
- [ ] `excel.deleteWorksheet` `{name: "Tmp"}` — removes the sheet.
- [ ] `excel.readRange` `{address: "A1:D10"}` — values match the fixture.
- [ ] `excel.writeRange` `{address: "F1:F3", values: [[1],[2],[3]]}` — read-back via `excel.readRange` matches.
- [ ] `excel.getSelectedRange` — returns the currently selected cell.
- [ ] `excel.setSelectedRange` `{address: "B2:C4"}` — `getSelectedRange` echoes back `B2:C4`.
- [ ] `excel.createTable` `{address: "A1:D10", name: "ParityTable"}` — table appears in workbook.
- [ ] `excel.runScript` `{script: "const w = context.workbook; w.load('name'); await context.sync(); return w.name;"}` — returns the workbook name.

## Known divergences from TS

- The Go binary does not auto-launch Excel via `office-addin-debugging` (PLAN.md §11 Open Question 7). Launch Excel manually with the debug port for now.
- `excel.runScript` accepts an arbitrary JS body. The TS repo did the same. Tighten via an allowlist later if security posture demands it.
- Telemetry (Clearcut, tiktoken token counting) is not present in v1 (PLAN.md §11 Open Question 6).
