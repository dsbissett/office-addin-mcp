package outlooktool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds all outlook.* tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(ReadItem())
	r.MustRegister(GetBody())
	r.MustRegister(SetBody())
	r.MustRegister(GetSubject())
	r.MustRegister(SetSubject())
	r.MustRegister(GetRecipients())
	r.MustRegister(RunScript())
}
