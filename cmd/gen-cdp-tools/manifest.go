package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest is the policy overlay that decides which CDP methods become tools
// and how they're shaped. Schemas come from the protocol JSON, never this
// file. See cdp/manifest.yaml for the live document.
type Manifest struct {
	Version  int                       `yaml:"version"`
	Defaults ManifestDefaults          `yaml:"defaults"`
	Domains  map[string]ManifestDomain `yaml:"domains"`
}

type ManifestDefaults struct {
	ToolPrefix string `yaml:"toolPrefix"`
	Scope      string `yaml:"scope"`
	Dangerous  bool   `yaml:"dangerous"`
}

type ManifestDomain struct {
	Scope      string                    `yaml:"scope"`      // overrides defaults.scope
	AutoEnable bool                      `yaml:"autoEnable"` // emit env.EnsureEnabled before each call
	Methods    map[string]ManifestMethod `yaml:"methods"`
}

type ManifestMethod struct {
	Dangerous           bool   `yaml:"dangerous"`
	BinaryField         string `yaml:"binaryField"`         // result field decoded to outputPath
	BinaryMimeType      string `yaml:"binaryMimeType"`      // mime type returned to caller alongside outputPath
	DescriptionOverride string `yaml:"descriptionOverride"` // replace upstream description
}

// LoadManifest parses the manifest YAML at path.
func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if m.Version != 1 {
		return nil, fmt.Errorf("manifest %s: unsupported version %d", path, m.Version)
	}
	if m.Defaults.ToolPrefix == "" {
		m.Defaults.ToolPrefix = "cdp"
	}
	if m.Defaults.Scope == "" {
		m.Defaults.Scope = "target"
	}
	return &m, nil
}

// EffectiveScope returns the scope for a domain, falling back to defaults.
func (m *Manifest) EffectiveScope(domain string) string {
	if d, ok := m.Domains[domain]; ok && d.Scope != "" {
		return d.Scope
	}
	return m.Defaults.Scope
}
