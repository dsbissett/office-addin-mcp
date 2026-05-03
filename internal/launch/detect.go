// Package launch implements manifest-aware Office add-in detection and launch
// helpers. It mirrors the Node-side reference at
// C:\Repos\excel-webview2-mcp\src\launch in Go: walk a working directory,
// identify the package.json + manifest.{xml,json} pair, then sideload Excel
// via office-addin-debugging with the WebView2 remote debugging port set.
package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// PackageManager identifies which Node package manager owns the project root.
type PackageManager string

const (
	PackageManagerNpm  PackageManager = "npm"
	PackageManagerPnpm PackageManager = "pnpm"
	PackageManagerYarn PackageManager = "yarn"
)

// ManifestKind disambiguates XML vs JSON Office add-in manifests.
type ManifestKind string

const (
	ManifestKindXML  ManifestKind = "xml"
	ManifestKindJSON ManifestKind = "json"
)

// DevServer describes the npm script that boots the add-in's web server and
// the port it advertises through package.json#config.dev_server_port.
type DevServer struct {
	Script string `json:"script"`
	Port   int    `json:"port"`
}

// Project is a detected Office add-in project (Excel, Word, Outlook,
// PowerPoint, OneNote — any host with a package.json + manifest pair).
type Project struct {
	Root           string         `json:"root"`
	ManifestPath   string         `json:"manifestPath"`
	ManifestKind   ManifestKind   `json:"manifestKind"`
	PackageManager PackageManager `json:"packageManager"`
	DevServer      *DevServer     `json:"devServer,omitempty"`
}

// ErrNoProject signals that no add-in project could be detected at or above
// the supplied directory.
var ErrNoProject = errors.New("launch: no Office add-in project detected")

// maxDetectDepth limits the upward walk searching for package.json so that a
// stray repo root never escapes the user's working directory.
const maxDetectDepth = 5

// DetectAddin walks up from cwd looking for a package.json adjacent to a
// manifest.{xml,json} that declares any Office add-in host. Returns
// ErrNoProject when none of the candidates match.
func DetectAddin(cwd string) (*Project, error) {
	root, err := findPackageRoot(cwd)
	if err != nil {
		return nil, err
	}
	pkg, err := readPackageJSON(filepath.Join(root, "package.json"))
	if err != nil {
		return nil, err
	}
	manifestPath, manifestKind, err := detectManifest(root)
	if err != nil {
		return nil, err
	}

	return &Project{
		Root:           root,
		ManifestPath:   manifestPath,
		ManifestKind:   manifestKind,
		PackageManager: detectPackageManager(root),
		DevServer:      detectDevServer(pkg),
	}, nil
}

func findPackageRoot(cwd string) (string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("launch: resolve cwd: %w", err)
	}
	current := abs
	for depth := 0; depth <= maxDetectDepth; depth++ {
		if pathExists(filepath.Join(current, "package.json")) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", ErrNoProject
}

type packageJSON struct {
	Scripts map[string]string `json:"scripts"`
	Config  struct {
		DevServerPort json.RawMessage `json:"dev_server_port"`
	} `json:"config"`
}

func readPackageJSON(path string) (*packageJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("launch: read %s: %w", path, err)
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("launch: parse %s: %w", path, err)
	}
	return &pkg, nil
}

func detectManifest(root string) (string, ManifestKind, error) {
	xmlPath := filepath.Join(root, "manifest.xml")
	if isOfficeXMLManifest(xmlPath) {
		return xmlPath, ManifestKindXML, nil
	}
	jsonPath := filepath.Join(root, "manifest.json")
	if isOfficeJSONManifest(jsonPath) {
		return jsonPath, ManifestKindJSON, nil
	}
	return "", "", ErrNoProject
}

var reOfficeApp = regexp.MustCompile(`(?i)<OfficeApp\b`)

// isOfficeXMLManifest accepts any well-formed XML add-in manifest regardless
// of which <Host Name="…"/> it declares (Workbook, Document, Presentation,
// Notebook, Mailbox).
func isOfficeXMLManifest(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return reOfficeApp.Match(data)
}

type jsonManifest struct {
	Extensions []struct {
		Requirements struct {
			Scopes []string `json:"scopes"`
		} `json:"requirements"`
	} `json:"extensions"`
}

// isOfficeJSONManifest accepts any unified Office manifest with at least one
// non-empty extension scope (workbook, document, mail, presentation, notebook).
func isOfficeJSONManifest(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var m jsonManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	for _, ext := range m.Extensions {
		if len(ext.Requirements.Scopes) > 0 {
			return true
		}
	}
	return false
}

func detectPackageManager(root string) PackageManager {
	if pathExists(filepath.Join(root, "pnpm-lock.yaml")) {
		return PackageManagerPnpm
	}
	if pathExists(filepath.Join(root, "yarn.lock")) {
		return PackageManagerYarn
	}
	return PackageManagerNpm
}

func detectDevServer(pkg *packageJSON) *DevServer {
	if pkg == nil || len(pkg.Scripts) == 0 {
		return nil
	}
	var script string
	for _, name := range []string{"dev-server", "dev:server", "dev"} {
		if v, ok := pkg.Scripts[name]; ok && v != "" {
			script = name
			break
		}
	}
	if script == "" {
		return nil
	}
	port, ok := decodePortValue(pkg.Config.DevServerPort)
	if !ok {
		return nil
	}
	return &DevServer{Script: script, Port: port}
}

// decodePortValue accepts either a JSON number or a numeric string for
// package.json#config.dev_server_port (the Yeoman generator emits a string).
func decodePortValue(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil && n > 0 {
		return n, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		var parsed int
		if _, err := fmt.Sscanf(s, "%d", &parsed); err == nil && parsed > 0 {
			return parsed, true
		}
	}
	return 0, false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
