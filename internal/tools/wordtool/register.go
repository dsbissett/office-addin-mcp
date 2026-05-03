package wordtool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds all word.* tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(ReadBody())
	r.MustRegister(WriteBody())
	r.MustRegister(ReadParagraphs())
	r.MustRegister(InsertParagraph())
	r.MustRegister(ReadSelection())
	r.MustRegister(SearchText())
	r.MustRegister(ReadProperties())
	r.MustRegister(RunScript())
}
