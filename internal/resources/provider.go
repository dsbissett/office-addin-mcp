package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// ReadResult is the content returned by a resource read.
type ReadResult struct {
	Text     string
	MIMEType string
}

// Provider dispatches resource reads to the appropriate tools.
type Provider struct {
	// Disp is the tools dispatcher.
	Disp *tools.Dispatcher
	// Endpoint returns the current default CDP endpoint.
	Endpoint func() webview2.Config
	// Cache is the persistent document discovery cache.
	Cache *doccache.Store
}

// Read dispatches the appropriate tool to read a resource URI and returns
// its content. Returns ResourceNotFound error if the URI is malformed or
// the underlying tool dispatch fails.
func (p *Provider) Read(ctx context.Context, uri string) (*ReadResult, error) {
	parsed, err := ParseURI(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid URI: %w", err)
	}

	var toolName string
	var params map[string]any

	switch parsed.Host {
	case "excel":
		toolName, params = p.readExcel(parsed)
	case "word":
		toolName, params = p.readWord(parsed)
	case "outlook":
		toolName, params = p.readOutlook(parsed)
	case "pp":
		toolName, params = p.readPowerPoint(parsed)
	case "onenote":
		toolName, params = p.readOneNote(parsed)
	default:
		return nil, fmt.Errorf("unknown host: %s", parsed.Host)
	}

	// Dispatch the tool.
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	req := tools.Request{
		Tool:     toolName,
		Params:   paramsJSON,
		Endpoint: p.Endpoint(),
	}
	env := p.Disp.Dispatch(ctx, req)

	if !env.OK {
		return nil, fmt.Errorf("tool dispatch failed: %s", env.Error.Message)
	}

	// Convert result to ReadResult.
	dataJSON, err := json.Marshal(env.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	return &ReadResult{
		Text:     string(dataJSON),
		MIMEType: "application/json",
	}, nil
}

// readExcel returns the tool and params for reading an Excel range.
// URI format: office://excel/Workbook/Sheet!Range
func (p *Provider) readExcel(parsed *ParsedURI) (string, map[string]any) {
	// Parts: [Workbook, Sheet!Range]
	// For simplicity, we'll just use the last part as the range.
	var rangeStr string
	if len(parsed.Parts) >= 2 {
		rangeStr = parsed.Parts[1]
	} else if len(parsed.Parts) == 1 {
		rangeStr = parsed.Parts[0]
	}

	return "excel.tabulateRegion", map[string]any{
		"range": rangeStr,
	}
}

// readWord returns the tool and params for reading a Word document or bookmark.
// URI format: office://word/Document or office://word/Document/bookmark/name
func (p *Provider) readWord(parsed *ParsedURI) (string, map[string]any) {
	// For Word, dispatch a script that reads the document body or a specific bookmark.
	// Parts: [Document] or [Document, "bookmark", BookmarkName]
	script := `
	await Word.run(async (ctx) => {
		ctx.document.body.load('text');
		await ctx.sync();
		return { text: ctx.document.body.text };
	});
	`

	// If Parts specifies a bookmark, inject the bookmark read logic.
	if len(parsed.Parts) >= 3 && parsed.Parts[1] == "bookmark" {
		bookmarkName := parsed.Parts[2]
		script = fmt.Sprintf(`
		await Word.run(async (ctx) => {
			const range = ctx.document.body.getBookmarkRange(%q);
			range.load('text');
			await ctx.sync();
			return { text: range.text };
		});
		`, bookmarkName)
	}

	return "word.runScript", map[string]any{
		"script": script,
	}
}

// readOutlook returns the tool and params for reading an Outlook folder.
// URI format: office://outlook/FolderName
func (p *Provider) readOutlook(parsed *ParsedURI) (string, map[string]any) {
	// Parts: [FolderName]
	folder := ""
	if len(parsed.Parts) > 0 {
		folder = parsed.Parts[0]
	}

	return "outlook.query", map[string]any{
		"folder": folder,
		"limit":  50,
	}
}

// readPowerPoint returns the tool and params for reading a PowerPoint slide.
// URI format: office://pp/Deck/slideN
func (p *Provider) readPowerPoint(parsed *ParsedURI) (string, map[string]any) {
	// Parts: [Deck, slideN]
	// Extract slide number from the second part (e.g., "slide2" -> 2)
	slideNum := 1
	if len(parsed.Parts) >= 2 {
		slideStr := parsed.Parts[1]
		if strings.HasPrefix(strings.ToLower(slideStr), "slide") {
			_, _ = fmt.Sscanf(slideStr, "slide%d", &slideNum)
		}
	}

	return "powerpoint.query", map[string]any{
		"slide": slideNum,
		"limit": 1,
	}
}

// readOneNote returns the tool and params for reading a OneNote page.
// URI format: office://onenote/Notebook/Section/Page
func (p *Provider) readOneNote(parsed *ParsedURI) (string, map[string]any) {
	// Parts: [Notebook, Section, Page]
	return "onenote.query", map[string]any{
		"limit": 1,
	}
}

// Fingerprint returns a short fingerprint of a resource for change detection.
// It dispatches the *.discover tool to get the document fingerprint.
func (p *Provider) Fingerprint(ctx context.Context, uri string) (string, error) {
	parsed, err := ParseURI(uri)
	if err != nil {
		return "", fmt.Errorf("invalid URI: %w", err)
	}

	toolName := parsed.Host + ".discover"

	// Dispatch with minimal params.
	req := tools.Request{
		Tool:     toolName,
		Params:   []byte("{}"),
		Endpoint: p.Endpoint(),
	}
	env := p.Disp.Dispatch(ctx, req)

	if !env.OK {
		return "", fmt.Errorf("discover failed: %s", env.Error.Message)
	}

	// Extract the fingerprint field from the result.
	// The discover payloads return {filePath, fingerprint, ...}
	result, ok := env.Data.(map[string]any)
	if !ok {
		return "", fmt.Errorf("unexpected result type from discover")
	}

	fingerprint, ok := result["fingerprint"].(string)
	if !ok {
		return "", fmt.Errorf("fingerprint not found in discover result")
	}

	return fingerprint, nil
}
