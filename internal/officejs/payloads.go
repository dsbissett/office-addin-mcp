// Package officejs runs Excel.js payloads inside a CDP target session. The
// payload sources live in internal/js (embedded) and are concatenated with a
// preamble that ensures Office.js is ready and surfaces structured errors.
package officejs

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"sync"

	"github.com/dsbissett/office-addin-mcp/internal/js"
)

// Requirement is a parsed `// @requires <set> <version>` directive declared
// at the top of a payload file. Surfaced via Requirements for diagnostics
// and tooling; runtime checks happen via the JS preamble's __requireSet.
type Requirement struct {
	Set     string
	Version string
}

const preambleFile = "_preamble.js"

var (
	loadOnce sync.Once
	loadErr  error

	preambleSrc     string
	payloadByName   map[string]string
	payloadRequires map[string][]Requirement
)

// Preload eagerly loads payloads so that init-time misconfigurations surface
// before the first tool call. Tests call this in TestMain; production wiring
// can rely on lazy load via getPayload.
func Preload() error {
	loadOnce.Do(load)
	return loadErr
}

func ensureLoaded() error {
	loadOnce.Do(load)
	return loadErr
}

func load() {
	payloadByName = map[string]string{}
	payloadRequires = map[string][]Requirement{}

	err := fs.WalkDir(js.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".js") {
			return nil
		}
		raw, err := js.FS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		src := string(raw)
		if path == preambleFile {
			preambleSrc = src
			return nil
		}
		toolName, err := fileToToolName(path)
		if err != nil {
			return err
		}
		payloadByName[toolName] = src
		payloadRequires[toolName] = parseRequires(src)
		return nil
	})
	if err != nil {
		loadErr = fmt.Errorf("officejs: load: %w", err)
		return
	}
	if preambleSrc == "" {
		loadErr = fmt.Errorf("officejs: %s missing from embed FS", preambleFile)
		return
	}
}

// getPayload returns the JS body for a tool name (e.g. "excel.readRange").
func getPayload(toolName string) (string, error) {
	if err := ensureLoaded(); err != nil {
		return "", err
	}
	body, ok := payloadByName[toolName]
	if !ok {
		return "", fmt.Errorf("officejs: no payload for tool %q", toolName)
	}
	return body, nil
}

// preamble returns the concatenated preamble source.
func preamble() (string, error) {
	if err := ensureLoaded(); err != nil {
		return "", err
	}
	return preambleSrc, nil
}

// Names returns the registered payload tool names.
func Names() []string {
	if err := ensureLoaded(); err != nil {
		return nil
	}
	out := make([]string, 0, len(payloadByName))
	for n := range payloadByName {
		out = append(out, n)
	}
	return out
}

// Requirements returns the `@requires` directives parsed from a payload.
func Requirements(toolName string) ([]Requirement, error) {
	if err := ensureLoaded(); err != nil {
		return nil, err
	}
	if _, ok := payloadByName[toolName]; !ok {
		return nil, fmt.Errorf("officejs: no payload for tool %q", toolName)
	}
	return payloadRequires[toolName], nil
}

// fileToToolName converts an embed path like "excel_read_range.js" to the
// tool name "excel.readRange".
func fileToToolName(path string) (string, error) {
	base := strings.TrimSuffix(path, ".js")
	idx := strings.IndexByte(base, '_')
	if idx <= 0 || idx == len(base)-1 {
		return "", fmt.Errorf("officejs: cannot derive tool name from %q (expected <domain>_<rest>.js)", path)
	}
	domain := base[:idx]
	rest := base[idx+1:]
	return domain + "." + camelize(rest), nil
}

// camelize: "read_range" → "readRange", "get_active_worksheet" → "getActiveWorksheet".
func camelize(s string) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 {
			b.WriteString(p)
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

var requireRE = regexp.MustCompile(`(?m)^\s*//\s*@requires\s+(\S+)\s+(\S+)`)

func parseRequires(src string) []Requirement {
	matches := requireRE.FindAllStringSubmatch(src, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]Requirement, 0, len(matches))
	for _, m := range matches {
		out = append(out, Requirement{Set: m[1], Version: m[2]})
	}
	return out
}
