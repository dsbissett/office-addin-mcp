package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Protocol mirrors the structure of Chrome's browser_protocol.json /
// js_protocol.json. We deliberately decode only the fields we use in code
// generation; ignored fields (Events, Dependencies, etc.) are dropped.
type Protocol struct {
	Domains []Domain `json:"domains"`
}

type Domain struct {
	Domain   string    `json:"domain"`
	Types    []Type    `json:"types"`
	Commands []Command `json:"commands"`
}

// Command is a CDP method (Page.navigate, etc.).
type Command struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Parameters  []Field `json:"parameters"`
	Returns     []Field `json:"returns"`
	Deprecated  bool    `json:"deprecated"`
}

// Type is an entry in domain.types — a type alias / object / enum.
type Type struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Type        string   `json:"type"` // string|integer|number|boolean|object|array
	Enum        []string `json:"enum"`
	Properties  []Field  `json:"properties"`
	Items       *Field   `json:"items"`
}

// Field is one parameter, return value, or object property. Has either Type
// or Ref set, not both.
type Field struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Optional    bool     `json:"optional"`
	Type        string   `json:"type,omitempty"`
	Ref         string   `json:"$ref,omitempty"` // "Type" (same domain) or "Domain.Type"
	Enum        []string `json:"enum,omitempty"`
	Items       *Field   `json:"items,omitempty"`
	Properties  []Field  `json:"properties,omitempty"`
}

// LoadProtocol decodes a single browser_protocol.json or js_protocol.json file.
func LoadProtocol(path string) (*Protocol, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read protocol %s: %w", path, err)
	}
	var p Protocol
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parse protocol %s: %w", path, err)
	}
	return &p, nil
}

// MergeProtocols folds the JS protocol into the browser protocol so callers
// see a single domain index. Domain names are unique across the two files in
// practice; if they ever collide, the second file wins (last-write-wins).
func MergeProtocols(p ...*Protocol) *Protocol {
	out := &Protocol{}
	for _, x := range p {
		out.Domains = append(out.Domains, x.Domains...)
	}
	return out
}

// Index returns a (domain -> Domain) map and a (domain.typeID -> Type) map.
func (p *Protocol) Index() (map[string]*Domain, map[string]*Type) {
	domains := map[string]*Domain{}
	types := map[string]*Type{}
	for i := range p.Domains {
		d := &p.Domains[i]
		domains[d.Domain] = d
		for j := range d.Types {
			t := &d.Types[j]
			types[d.Domain+"."+t.ID] = t
		}
	}
	return domains, types
}

// FindCommand returns the named command in the domain, or nil.
func (d *Domain) FindCommand(name string) *Command {
	for i := range d.Commands {
		if d.Commands[i].Name == name {
			return &d.Commands[i]
		}
	}
	return nil
}
