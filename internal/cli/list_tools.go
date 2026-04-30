package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// RunListTools prints the registered tools as a single JSON document on
// stdout. Each entry includes name, description, and the tool's JSON Schema.
func RunListTools(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "list-tools: takes no arguments")
		return 2
	}
	return runListToolsWith(DefaultRegistry(), stdout, stderr)
}

func runListToolsWith(reg *tools.Registry, stdout, stderr io.Writer) int {
	type item struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Schema      json.RawMessage `json:"schema"`
	}
	registered := reg.List()
	out := struct {
		EnvelopeVersion string `json:"envelopeVersion"`
		Tools           []item `json:"tools"`
	}{
		EnvelopeVersion: tools.EnvelopeVersion,
		Tools:           make([]item, 0, len(registered)),
	}
	for _, t := range registered {
		out.Tools = append(out.Tools, item{
			Name:        t.Name,
			Description: t.Description,
			Schema:      t.Schema,
		})
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(stderr, "list-tools: encode: %v\n", err)
		return 1
	}
	return 0
}
