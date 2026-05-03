package addin

// StandardRequirementSets is the canonical list of requirement sets the
// addin.contextInfo tool probes against Office.context.requirements when no
// custom set list is supplied by the caller. The values mirror the reference
// excel-webview2-mcp implementation; adding new sets here is safe.
var StandardRequirementSets = []RequirementSet{
	{Name: "ExcelApi", MinVersion: "1.1"},
	{Name: "ExcelApi", MinVersion: "1.4"},
	{Name: "ExcelApi", MinVersion: "1.7"},
	{Name: "ExcelApi", MinVersion: "1.9"},
	{Name: "ExcelApi", MinVersion: "1.11"},
	{Name: "ExcelApi", MinVersion: "1.13"},
	{Name: "ExcelApi", MinVersion: "1.14"},
	{Name: "ExcelApi", MinVersion: "1.15"},
	{Name: "ExcelApi", MinVersion: "1.16"},
	{Name: "ExcelApi", MinVersion: "1.17"},
	{Name: "ExcelApiOnline", MinVersion: "1.1"},
	{Name: "SharedRuntime", MinVersion: "1.1"},
	{Name: "DialogApi", MinVersion: "1.1"},
	{Name: "DialogApi", MinVersion: "1.2"},
	{Name: "RibbonApi", MinVersion: "1.1"},
	{Name: "IdentityAPI", MinVersion: "1.3"},
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
}

// MergeRequirementSets returns a deduplicated union of base and extras keyed
// by (Name, MinVersion). Used by addin.contextInfo to add manifest-declared
// sets on top of the standard list.
func MergeRequirementSets(base, extras []RequirementSet) []RequirementSet {
	out := append([]RequirementSet(nil), base...)
	for _, e := range extras {
		seen := false
		for _, b := range out {
			if b.Name == e.Name && b.MinVersion == e.MinVersion {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, e)
		}
	}
	return out
}
