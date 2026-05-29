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
	addr := extractParamString(params, "address")
	if addr != "" {
		errEnv.Details["failing_address"] = addr
	}
	enrichByHost(ctx, env, hostFromTool(toolName), errEnv, params, addr)
}

// enrichByHost routes an Office.js error to the host-specific enricher.
func enrichByHost(ctx context.Context, env *RunEnv, host string, errEnv *EnvelopeError, params json.RawMessage, addr string) {
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
		enrichExcelInvalidAddress(errEnv, addr)
	}

	if errEnv.Code != "ItemNotFound" {
		return
	}
	enrichExcelItemNotFound(ctx, env, errEnv, params, addr)
}

// enrichExcelInvalidAddress folds parsed-address bounds into the error details
// and sets the invalid-address recovery hint when one is not already present.
func enrichExcelInvalidAddress(errEnv *EnvelopeError, addr string) {
	info := analyzeAddress(addr)
	if len(info) == 0 {
		return
	}
	for k, v := range info {
		errEnv.Details[k] = v
	}
	if errEnv.RecoveryHint == "" {
		errEnv.RecoveryHint = "Range address rejected as invalid. Inspect parsed bounds in details and retry with a valid address."
	}
}

// enrichExcelItemNotFound looks up the available sheets and nearest-name
// suggestions for an ItemNotFound failure, setting the matching recovery hint.
func enrichExcelItemNotFound(ctx context.Context, env *RunEnv, errEnv *EnvelopeError, params json.RawMessage, addr string) {
	sheets, source := lookupExcelSheets(ctx, env, params)
	if len(sheets) == 0 {
		if errEnv.RecoveryHint == "" {
			errEnv.RecoveryHint = "Sheet or range not found. Call excel.discover (or excel.summarizeWorkbook) to list available sheets, then retry."
		}
		return
	}
	errEnv.Details["available_sheets"] = sheets
	errEnv.Details["available_sheets_source"] = source
	addNearestSheetSuggestions(errEnv, addr, sheets)
	if errEnv.RecoveryHint == "" {
		errEnv.RecoveryHint = "Sheet or range not found. Compare your address against available_sheets and nearest_name_suggestions; retry with a corrected address."
	}
}

// addNearestSheetSuggestions records up to three closest sheet names to the
// failing address (preferring its sheet prefix) when any are within range.
func addNearestSheetSuggestions(errEnv *EnvelopeError, addr string, sheets []string) {
	target := sheetFromAddress(addr)
	if target == "" {
		target = addr
	}
	if target == "" {
		return
	}
	if matches := nearestNames(target, sheets, 3); len(matches) > 0 {
		errEnv.Details["nearest_name_suggestions"] = matches
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
	if !outlookModeMismatch(errEnv) {
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

// outlookModeMismatch reports whether an Outlook error looks like a
// compose-vs-read item-mode mismatch worth enriching with the live item mode.
func outlookModeMismatch(errEnv *EnvelopeError) bool {
	return outlookMessageHints(errEnv.Message) || outlookCodeHints(errEnv.Code)
}

func outlookMessageHints(message string) bool {
	msg := strings.ToLower(message)
	for _, sub := range []string{"compose", "read mode", "item mode", "currently selected"} {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	return false
}

func outlookCodeHints(code string) bool {
	switch strings.ToUpper(code) {
	case "INVALIDOPERATION", "ITEMNOTFOUND":
		return true
	}
	return false
}

// lookupExcelSheets returns the available sheet names. Doccache wins when an
// entry is present; otherwise a one-shot excel.listWorksheets call against the
// already-attached target. Returns the sheets and a label naming the source.
func lookupExcelSheets(ctx context.Context, env *RunEnv, params json.RawMessage) ([]string, string) {
	if names := cachedExcelSheets(env); len(names) > 0 {
		return names, "doccache"
	}
	raw, err := runDiagnosticsPayload(ctx, env, params, "excel.listWorksheets", nil)
	if err != nil {
		return nil, ""
	}
	if names := sheetsFromCacheData(raw); len(names) > 0 {
		return names, "live"
	}
	return nil, ""
}

// cachedExcelSheets returns sheet names from the first doccache entry that
// carries any, or nil when the cache is unavailable or empty.
func cachedExcelSheets(env *RunEnv) []string {
	if env == nil || env.DocCache == nil {
		return nil
	}
	for _, e := range env.DocCache.List("excel") {
		if names := sheetsFromCacheData(e.Data); len(names) > 0 {
			return names
		}
	}
	return nil
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
	if n, ok := cachedSlideCount(env); ok {
		return n, true
	}
	raw, err := runDiagnosticsPayload(ctx, env, params, "powerpoint.discover", nil)
	if err != nil {
		return 0, false
	}
	return slideCountFromCacheData(raw)
}

// cachedSlideCount returns the slide count from the first doccache entry that
// carries one, or (0,false) when the cache is unavailable or empty.
func cachedSlideCount(env *RunEnv) (int, bool) {
	if env == nil || env.DocCache == nil {
		return 0, false
	}
	for _, e := range env.DocCache.List("powerpoint") {
		if n, ok := slideCountFromCacheData(e.Data); ok {
			return n, true
		}
	}
	return 0, false
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
	if mode, ok := cachedItemMode(env); ok {
		return mode, true
	}
	raw, err := runDiagnosticsPayload(ctx, env, params, "outlook.discover", nil)
	if err != nil {
		return "", false
	}
	return itemModeFromCacheData(raw)
}

// cachedItemMode returns the host item mode from the first doccache entry that
// carries one, or ("",false) when the cache is unavailable or empty.
func cachedItemMode(env *RunEnv) (string, bool) {
	if env == nil || env.DocCache == nil {
		return "", false
	}
	for _, e := range env.DocCache.List("outlook") {
		if mode, ok := itemModeFromCacheData(e.Data); ok {
			return mode, true
		}
	}
	return "", false
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
	return unquoteSheetName(addr[:bang])
}

// unquoteSheetName strips the surrounding single quotes from a sheet name and
// collapses doubled quotes, returning the input unchanged when not quoted.
func unquoteSheetName(name string) string {
	if !isQuoted(name) {
		return name
	}
	return strings.ReplaceAll(name[1:len(name)-1], "''", "'")
}

func isQuoted(s string) bool {
	return len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\''
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
	checkAddressBounds(out, m[1], m[2])
	if m[3] != "" {
		parsed["end_column"] = strings.ToUpper(m[3])
		parsed["end_row"] = mustAtoi(m[4])
		checkAddressBounds(out, m[3], m[4])
	}
	out["parsed_address"] = parsed
	return out
}

// checkAddressBounds records out-of-bounds column/row markers into out for a
// single column-letters / row-digits pair. Later pairs overwrite earlier
// markers, matching the original start-then-end evaluation order.
func checkAddressBounds(out map[string]any, colLetters, rowDigits string) {
	if columnIndex(colLetters) > excelMaxColumn {
		out["column_out_of_bounds"] = strings.ToUpper(colLetters)
		out["max_column"] = "XFD"
	}
	if r := mustAtoi(rowDigits); r > excelMaxRow {
		out["row_out_of_bounds"] = r
		out["max_row"] = excelMaxRow
	}
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
	scoredOut := scoreNames(query, names)
	sort.SliceStable(scoredOut, func(i, j int) bool { return scoredOut[i].dist < scoredOut[j].dist })
	return topNames(scoredOut, limit)
}

// topNames returns the names of the first limit entries (or fewer) in order.
func topNames(scored []scoredName, limit int) []string {
	if limit > len(scored) {
		limit = len(scored)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, scored[i].name)
	}
	return out
}

type scoredName struct {
	name string
	dist int
	idx  int
}

// scoreNames computes the edit distance of each name to query, dropping any
// whose distance exceeds half the query length (min 2) as an obvious mismatch.
func scoreNames(query string, names []string) []scoredName {
	q := strings.ToLower(query)
	maxDist := len(query) / 2
	if maxDist < 2 {
		maxDist = 2
	}
	out := make([]scoredName, 0, len(names))
	for i, n := range names {
		d := levenshtein(q, strings.ToLower(n))
		if d > maxDist {
			continue
		}
		out = append(out, scoredName{name: n, dist: d, idx: i})
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
		levenshteinRow(prev, curr, ar[i-1], br)
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

// levenshteinRow fills curr[1..len(br)] for one source rune ac, given the
// previous DP row in prev.
func levenshteinRow(prev, curr []int, ac rune, br []rune) {
	for j := 1; j <= len(br); j++ {
		cost := 1
		if ac == br[j-1] {
			cost = 0
		}
		curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
	}
}

// min3 returns the smallest of three ints.
func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
