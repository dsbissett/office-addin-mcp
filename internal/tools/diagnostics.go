package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/officejs"
)

// classifyOfficeJSErr enriches an Office.js EnvelopeError with structured
// recovery hints — available sheet names, parsed address bounds, slide count,
// item mode, etc. — so an AI client can self-correct without re-deriving the
// state from a follow-up tool call. Bounded to one extra CDP round-trip per
// error (a single payload call against the already-attached target).
//
// It mutates errEnv in place. Safe to call when env is nil or env.Attach is
// nil (NoSession path) — the live-lookup branches simply skip and fall back to
// doccache or no-op.
func classifyOfficeJSErr(ctx context.Context, env *RunEnv, toolName string, params json.RawMessage, errEnv *EnvelopeError) {
	if errEnv == nil || errEnv.Category != CategoryOfficeJS {
		return
	}
	if errEnv.Details == nil {
		errEnv.Details = map[string]any{}
	}
	host := hostFromTool(toolName)
	addr := extractParamString(params, "address")
	if addr != "" {
		errEnv.Details["failing_address"] = addr
	}

	switch host {
	case "excel":
		enrichExcel(ctx, env, errEnv, params, addr)
	case "powerpoint":
		enrichPowerPoint(ctx, env, errEnv, params)
	case "outlook":
		enrichOutlook(ctx, env, errEnv, params)
	}
}

func enrichExcel(ctx context.Context, env *RunEnv, errEnv *EnvelopeError, params json.RawMessage, addr string) {
	switch errEnv.Code {
	case "ItemNotFound", "InvalidArgument":
	default:
		return
	}

	if errEnv.Code == "InvalidArgument" && addr != "" {
		if info := analyzeAddress(addr); len(info) > 0 {
			for k, v := range info {
				errEnv.Details[k] = v
			}
			if errEnv.RecoveryHint == "" {
				errEnv.RecoveryHint = "Range address rejected as invalid. Inspect parsed bounds in details and retry with a valid address."
			}
		}
	}

	if errEnv.Code != "ItemNotFound" {
		return
	}

	sheets, source := lookupExcelSheets(ctx, env, params)
	if len(sheets) == 0 {
		if errEnv.RecoveryHint == "" {
			errEnv.RecoveryHint = "Sheet or range not found. Call excel.discover (or excel.summarizeWorkbook) to list available sheets, then retry."
		}
		return
	}
	errEnv.Details["available_sheets"] = sheets
	errEnv.Details["available_sheets_source"] = source

	target := sheetFromAddress(addr)
	if target == "" {
		target = addr
	}
	if target != "" {
		if matches := nearestNames(target, sheets, 3); len(matches) > 0 {
			errEnv.Details["nearest_name_suggestions"] = matches
		}
	}
	if errEnv.RecoveryHint == "" {
		errEnv.RecoveryHint = "Sheet or range not found. Compare your address against available_sheets and nearest_name_suggestions; retry with a corrected address."
	}
}

func enrichPowerPoint(ctx context.Context, env *RunEnv, errEnv *EnvelopeError, params json.RawMessage) {
	switch errEnv.Code {
	case "ItemNotFound", "InvalidArgument":
	default:
		return
	}
	count, ok := lookupPowerPointSlideCount(ctx, env, params)
	if !ok {
		return
	}
	errEnv.Details["slide_count"] = count
	if errEnv.RecoveryHint == "" {
		errEnv.RecoveryHint = fmt.Sprintf("PowerPoint slide reference is out of range. Presentation has %d slide(s); retry with a 1-based slide index in range.", count)
	}
}

func enrichOutlook(ctx context.Context, env *RunEnv, errEnv *EnvelopeError, params json.RawMessage) {
	msg := strings.ToLower(errEnv.Message)
	codeUpper := strings.ToUpper(errEnv.Code)
	hits := strings.Contains(msg, "compose") ||
		strings.Contains(msg, "read mode") ||
		strings.Contains(msg, "item mode") ||
		strings.Contains(msg, "currently selected") ||
		codeUpper == "INVALIDOPERATION" ||
		codeUpper == "ITEMNOTFOUND"
	if !hits {
		return
	}
	mode, ok := lookupOutlookItemMode(ctx, env, params)
	if !ok {
		return
	}
	errEnv.Details["item_mode"] = mode
	if errEnv.RecoveryHint == "" {
		errEnv.RecoveryHint = fmt.Sprintf("Outlook item is in %q mode; the requested operation likely requires the other mode. Switch to a matching item or use an appropriate compose/read tool.", mode)
	}
}

// lookupExcelSheets returns the available sheet names. Doccache wins when an
// entry is present; otherwise a one-shot excel.listWorksheets call against the
// already-attached target. Returns the sheets and a label naming the source.
func lookupExcelSheets(ctx context.Context, env *RunEnv, params json.RawMessage) ([]string, string) {
	if env != nil && env.DocCache != nil {
		for _, e := range env.DocCache.List("excel") {
			if names := sheetsFromCacheData(e.Data); len(names) > 0 {
				return names, "doccache"
			}
		}
	}
	raw, err := runDiagnosticsPayload(ctx, env, params, "excel.listWorksheets", nil)
	if err != nil {
		return nil, ""
	}
	var out struct {
		Worksheets []struct {
			Name string `json:"name"`
		} `json:"worksheets"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, ""
	}
	names := make([]string, 0, len(out.Worksheets))
	for _, w := range out.Worksheets {
		if w.Name != "" {
			names = append(names, w.Name)
		}
	}
	if len(names) == 0 {
		return nil, ""
	}
	return names, "live"
}

func sheetsFromCacheData(data json.RawMessage) []string {
	if len(data) == 0 {
		return nil
	}
	var out struct {
		Worksheets []struct {
			Name string `json:"name"`
		} `json:"worksheets"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	names := make([]string, 0, len(out.Worksheets))
	for _, w := range out.Worksheets {
		if w.Name != "" {
			names = append(names, w.Name)
		}
	}
	return names
}

func lookupPowerPointSlideCount(ctx context.Context, env *RunEnv, params json.RawMessage) (int, bool) {
	if env != nil && env.DocCache != nil {
		for _, e := range env.DocCache.List("powerpoint") {
			if n, ok := slideCountFromCacheData(e.Data); ok {
				return n, true
			}
		}
	}
	raw, err := runDiagnosticsPayload(ctx, env, params, "powerpoint.discover", nil)
	if err != nil {
		return 0, false
	}
	var out struct {
		SlideCount int `json:"slideCount"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, false
	}
	if out.SlideCount <= 0 {
		return 0, false
	}
	return out.SlideCount, true
}

func slideCountFromCacheData(data json.RawMessage) (int, bool) {
	if len(data) == 0 {
		return 0, false
	}
	var out struct {
		SlideCount int `json:"slideCount"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, false
	}
	if out.SlideCount <= 0 {
		return 0, false
	}
	return out.SlideCount, true
}

func lookupOutlookItemMode(ctx context.Context, env *RunEnv, params json.RawMessage) (string, bool) {
	if env != nil && env.DocCache != nil {
		for _, e := range env.DocCache.List("outlook") {
			if mode, ok := itemModeFromCacheData(e.Data); ok {
				return mode, true
			}
		}
	}
	raw, err := runDiagnosticsPayload(ctx, env, params, "outlook.discover", nil)
	if err != nil {
		return "", false
	}
	mode, ok := itemModeFromCacheData(raw)
	return mode, ok
}

func itemModeFromCacheData(data json.RawMessage) (string, bool) {
	if len(data) == 0 {
		return "", false
	}
	var out struct {
		HostMode string `json:"hostMode"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", false
	}
	if out.HostMode == "" {
		return "", false
	}
	return out.HostMode, true
}

// runDiagnosticsPayload runs a small Office.js payload through the existing
// session, reusing the cached selector when no targetId/urlPattern was passed.
// Returns the raw payload result, the *officejs.OfficeError, or any transport
// error. Returns an error for NoSession tools (env.Attach == nil).
func runDiagnosticsPayload(ctx context.Context, env *RunEnv, params json.RawMessage, payload string, args any) (json.RawMessage, error) {
	if env == nil || env.Attach == nil {
		return nil, fmt.Errorf("diagnostics: no attach helper")
	}
	sel := TargetSelector{
		TargetID:   extractParamString(params, "targetId"),
		URLPattern: extractParamString(params, "urlPattern"),
	}
	att, err := env.Attach(ctx, sel)
	if err != nil {
		return nil, err
	}
	exec := officejs.New(att.Conn, att.SessionID)
	return exec.Run(ctx, payload, args)
}

func hostFromTool(name string) string {
	if i := strings.IndexByte(name, '.'); i > 0 {
		return name[:i]
	}
	return ""
}

func extractParamString(params json.RawMessage, key string) string {
	if len(params) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(params, &m); err != nil {
		return ""
	}
	raw, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// sheetFromAddress pulls the sheet portion from "Sheet1!A1:B2" or
// "'Quoted Name'!A1". Returns "" when the address has no sheet prefix.
func sheetFromAddress(addr string) string {
	if addr == "" {
		return ""
	}
	bang := strings.LastIndexByte(addr, '!')
	if bang <= 0 {
		return ""
	}
	name := addr[:bang]
	if len(name) >= 2 && name[0] == '\'' && name[len(name)-1] == '\'' {
		name = strings.ReplaceAll(name[1:len(name)-1], "''", "'")
	}
	return name
}

var rangeRE = regexp.MustCompile(`^([A-Za-z]+)([0-9]+)(?::([A-Za-z]+)([0-9]+))?$`)

const (
	excelMaxColumn = 16384 // XFD
	excelMaxRow    = 1048576
)

// analyzeAddress parses a range body (after stripping the optional sheet
// prefix) into a parsed_address detail map and reports out-of-bounds
// column/row indexes. Returns nil when the address is unparseable.
func analyzeAddress(addr string) map[string]any {
	body := addr
	if bang := strings.LastIndexByte(addr, '!'); bang > 0 {
		body = addr[bang+1:]
	}
	body = strings.TrimSpace(body)
	body = strings.ReplaceAll(body, "$", "")
	m := rangeRE.FindStringSubmatch(body)
	if m == nil {
		return nil
	}
	out := map[string]any{}
	parsed := map[string]any{
		"start_column": strings.ToUpper(m[1]),
		"start_row":    mustAtoi(m[2]),
	}
	if startCol := columnIndex(m[1]); startCol > excelMaxColumn {
		out["column_out_of_bounds"] = strings.ToUpper(m[1])
		out["max_column"] = "XFD"
	}
	if r := mustAtoi(m[2]); r > excelMaxRow {
		out["row_out_of_bounds"] = r
		out["max_row"] = excelMaxRow
	}
	if m[3] != "" {
		parsed["end_column"] = strings.ToUpper(m[3])
		parsed["end_row"] = mustAtoi(m[4])
		if endCol := columnIndex(m[3]); endCol > excelMaxColumn {
			out["column_out_of_bounds"] = strings.ToUpper(m[3])
			out["max_column"] = "XFD"
		}
		if r := mustAtoi(m[4]); r > excelMaxRow {
			out["row_out_of_bounds"] = r
			out["max_row"] = excelMaxRow
		}
	}
	out["parsed_address"] = parsed
	return out
}

func columnIndex(letters string) int {
	letters = strings.ToUpper(letters)
	idx := 0
	for _, ch := range letters {
		if ch < 'A' || ch > 'Z' {
			return 0
		}
		idx = idx*26 + int(ch-'A'+1)
	}
	return idx
}

func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// nearestNames returns up to limit names ordered by ascending edit distance to
// query. Names tied at the same distance keep their input order. Names beyond
// twice the query length are filtered as obvious mismatches.
func nearestNames(query string, names []string, limit int) []string {
	if query == "" || len(names) == 0 || limit <= 0 {
		return nil
	}
	type scored struct {
		name string
		dist int
		idx  int
	}
	q := strings.ToLower(query)
	cap := len(query) / 2
	if cap < 2 {
		cap = 2
	}
	scoredOut := make([]scored, 0, len(names))
	for i, n := range names {
		d := levenshtein(q, strings.ToLower(n))
		if d > cap {
			continue
		}
		scoredOut = append(scoredOut, scored{name: n, dist: d, idx: i})
	}
	sort.SliceStable(scoredOut, func(i, j int) bool { return scoredOut[i].dist < scoredOut[j].dist })
	if limit > len(scoredOut) {
		limit = len(scoredOut)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, scoredOut[i].name)
	}
	return out
}

func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := 0; j <= len(br); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = del
			if ins < curr[j] {
				curr[j] = ins
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}
