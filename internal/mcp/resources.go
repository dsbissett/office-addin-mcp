package mcp

import (
	"context"
	"fmt"
	"log/slog"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dsbissett/office-addin-mcp/internal/resources"
)

// registerResources adds all five host resource templates to the SDK server.
func registerResources(s *sdk.Server, provider *resources.Provider) {
	hosts := []struct {
		name        string
		description string
	}{
		{"excel", "Excel spreadsheet ranges and workbooks"},
		{"word", "Word documents and bookmarks"},
		{"outlook", "Outlook mailbox folders and items"},
		{"pp", "PowerPoint presentations and slides"},
		{"onenote", "OneNote notebooks, sections, and pages"},
	}

	for _, host := range hosts {
		// Create a handler closure that captures the host name.
		hostName := host.name // capture for closure
		handler := func(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			return resourceHandler(ctx, req, provider)
		}

		// Register the resource template using {+path} to capture everything after the host.
		tmpl := &sdk.ResourceTemplate{
			URITemplate: fmt.Sprintf("office://%s/{+path}", hostName),
			Name:        fmt.Sprintf("%s Resources", hostName),
			Description: host.description,
			MIMEType:    "application/json",
		}

		s.AddResourceTemplate(tmpl, handler)
		slog.Debug("registered resource template", "uri_template", tmpl.URITemplate)
	}
}

// resourceHandler reads a resource by dispatching to the provider.
func resourceHandler(ctx context.Context, req *sdk.ReadResourceRequest, provider *resources.Provider) (*sdk.ReadResourceResult, error) {
	uri := req.Params.URI

	// Parse and read the resource.
	result, err := provider.Read(ctx, uri)
	if err != nil {
		slog.Debug("resource read failed", "uri", uri, "error", err)
		return nil, sdk.ResourceNotFoundError(uri)
	}

	return &sdk.ReadResourceResult{
		Contents: []*sdk.ResourceContents{
			{
				URI:      uri,
				MIMEType: result.MIMEType,
				Text:     result.Text,
			},
		},
	}, nil
}
