package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Resolver dereferences $ref names against the indexed protocol.
type Resolver struct {
	Types map[string]*Type // key: "Domain.TypeID"
}

// resolve looks up a $ref. ref may be bare ("FrameId" — same domain) or
// dotted ("Network.LoaderId" — cross-domain). currentDomain is used to expand
// the bare form.
func (r *Resolver) resolve(currentDomain, ref string) (*Type, error) {
	key := ref
	if !strings.Contains(ref, ".") {
		key = currentDomain + "." + ref
	}
	t, ok := r.Types[key]
	if !ok {
		return nil, fmt.Errorf("unknown $ref %q (resolved to %q)", ref, key)
	}
	return t, nil
}

// FieldSchema is the JSON Schema for one Field, plus the matching Go field
// metadata for the generated params struct.
type FieldSchema struct {
	Name        string // CDP camelCase name, e.g. "url"
	GoName      string // exported Go name, e.g. "URL"
	GoType      string // e.g. "string", "int64", "json.RawMessage"
	JSONTag     string // e.g. `json:"url"` or `json:"url,omitempty"`
	Required    bool
	Description string
	SchemaJSON  json.RawMessage // the property's JSON Schema fragment
}

// FieldsToSchema renders a list of CDP Fields into a top-level params JSON
// Schema (object with properties + required) plus per-field Go metadata.
//
// extraSelector, if true, appends targetId/urlPattern selector properties to
// the schema and Go struct (target-scoped tools). The selector fields are
// MCP-only — the dispatcher consumes them via env.Attach and they never
// reach CDP.
//
// extraOutputPath, if true, appends an outputPath property used by binary-
// field tools (Page.captureScreenshot, etc.) to redirect the base64 payload
// to disk. Also MCP-only.
func FieldsToSchema(r *Resolver, currentDomain, methodTitle string, fields []Field, extraSelector, extraOutputPath bool) (
	schema json.RawMessage, gofields []FieldSchema, err error,
) {
	props := map[string]json.RawMessage{}
	required := []string{}

	for _, f := range fields {
		propSchema, err := fieldToSchema(r, currentDomain, f)
		if err != nil {
			return nil, nil, fmt.Errorf("field %q: %w", f.Name, err)
		}
		props[f.Name] = propSchema

		fs := FieldSchema{
			Name:        f.Name,
			GoName:      goName(f.Name),
			GoType:      fieldGoType(r, currentDomain, f),
			Required:    !f.Optional,
			Description: f.Description,
			SchemaJSON:  propSchema,
		}
		if f.Optional {
			fs.JSONTag = fmt.Sprintf(`json:"%s,omitempty"`, f.Name)
		} else {
			fs.JSONTag = fmt.Sprintf(`json:"%s"`, f.Name)
			required = append(required, f.Name)
		}
		gofields = append(gofields, fs)
	}

	if extraSelector {
		// targetId / urlPattern are MCP-tool-only selectors used by env.Attach;
		// they never reach CDP. Emit them as optional.
		for _, sel := range selectorFields() {
			props[sel.Name] = sel.SchemaJSON
			gofields = append(gofields, sel)
		}
	}
	if extraOutputPath {
		op := outputPathField()
		props[op.Name] = op.SchemaJSON
		gofields = append(gofields, op)
	}

	// Render with sorted property keys for determinism.
	out := bytes.NewBufferString("{")
	out.WriteString(`"$schema":"https://json-schema.org/draft/2020-12/schema","title":"`)
	out.WriteString(methodTitle)
	out.WriteString(`","type":"object","properties":{`)
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i > 0 {
			out.WriteString(",")
		}
		out.WriteString(strconv.Quote(k))
		out.WriteString(":")
		out.Write(props[k])
	}
	out.WriteString("}")
	if len(required) > 0 {
		sort.Strings(required)
		out.WriteString(`,"required":[`)
		for i, k := range required {
			if i > 0 {
				out.WriteString(",")
			}
			out.WriteString(strconv.Quote(k))
		}
		out.WriteString("]")
	}
	out.WriteString(`,"additionalProperties":false}`)
	return out.Bytes(), gofields, nil
}

// fieldToSchema turns one CDP Field into a JSON Schema fragment for that
// property. Resolves $ref recursively.
func fieldToSchema(r *Resolver, currentDomain string, f Field) (json.RawMessage, error) {
	props := map[string]any{}
	if f.Description != "" {
		props["description"] = f.Description
	}
	if err := mergeTypeIntoSchema(r, currentDomain, f.Type, f.Ref, f.Enum, f.Items, f.Properties, props); err != nil {
		return nil, err
	}
	return marshalDeterministic(props)
}

// mergeTypeIntoSchema fills `out` with the JSON Schema keywords for a CDP
// type/$ref combination. f-level properties (description, optional) are
// applied by the caller.
func mergeTypeIntoSchema(r *Resolver, currentDomain, cdpType, ref string, enum []string, items *Field, properties []Field, out map[string]any) error {
	if ref != "" {
		t, err := r.resolve(currentDomain, ref)
		if err != nil {
			return err
		}
		// Cross-domain refs resolve in their *own* domain for further lookups.
		nextDomain := currentDomain
		if i := strings.IndexByte(ref, '.'); i > 0 {
			nextDomain = ref[:i]
		}
		return mergeTypeIntoSchema(r, nextDomain, t.Type, "", t.Enum, t.Items, t.Properties, out)
	}
	switch cdpType {
	case "string":
		out["type"] = "string"
	case "integer":
		out["type"] = "integer"
	case "number":
		out["type"] = "number"
	case "boolean":
		out["type"] = "boolean"
	case "any", "":
		// any: no type constraint. Empty cdpType also reaches here for
		// type defs we couldn't fully resolve; permissive is the safe choice.
	case "binary":
		// CDP binary is base64-encoded into a string field.
		out["type"] = "string"
		out["contentEncoding"] = "base64"
	case "object":
		out["type"] = "object"
		if len(properties) > 0 {
			subProps := map[string]any{}
			var req []string
			for _, p := range properties {
				sub, err := fieldToSchema(r, currentDomain, p)
				if err != nil {
					return err
				}
				var decoded map[string]any
				if err := json.Unmarshal(sub, &decoded); err != nil {
					return err
				}
				subProps[p.Name] = decoded
				if !p.Optional {
					req = append(req, p.Name)
				}
			}
			out["properties"] = subProps
			if len(req) > 0 {
				sort.Strings(req)
				out["required"] = req
			}
		}
	case "array":
		out["type"] = "array"
		if items != nil {
			itemSchema, err := fieldToSchema(r, currentDomain, *items)
			if err != nil {
				return err
			}
			var decoded any
			if err := json.Unmarshal(itemSchema, &decoded); err != nil {
				return err
			}
			out["items"] = decoded
		}
	default:
		return fmt.Errorf("unknown CDP type %q", cdpType)
	}
	if len(enum) > 0 {
		out["enum"] = append([]string(nil), enum...)
	}
	return nil
}

// fieldGoType returns the Go type to use in the generated params struct for a
// CDP field. Pointer-to-bool/int is avoided — we use the value type and rely
// on the JSON Schema to enforce required-ness.
func fieldGoType(r *Resolver, currentDomain string, f Field) string {
	cdpType := f.Type
	if f.Ref != "" {
		t, err := r.resolve(currentDomain, f.Ref)
		if err != nil {
			// Unresolved refs are exceedingly rare and treated as opaque.
			return "json.RawMessage"
		}
		cdpType = t.Type
	}
	switch cdpType {
	case "string", "binary":
		return "string"
	case "integer":
		return "int64"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "object", "array", "any", "":
		return "json.RawMessage"
	}
	return "json.RawMessage"
}

// outputPathField is the optional outputPath param appended to every binary-
// field tool. Setting it redirects the base64 payload to disk; the envelope
// returns BinaryOutput metadata instead of the raw bytes.
func outputPathField() FieldSchema {
	return FieldSchema{
		Name:    "outputPath",
		GoName:  "OutputPath",
		GoType:  "string",
		JSONTag: `json:"outputPath,omitempty"`,
		SchemaJSON: json.RawMessage(
			`{"type":"string","description":"Filesystem path to write the binary payload to. When set, the envelope returns {path,sizeBytes,mimeType} instead of the raw base64 field."}`,
		),
	}
}

// selectorFields returns the targetId / urlPattern fields appended to every
// target-scoped tool. Their JSON Schema fragments are stable and shared.
func selectorFields() []FieldSchema {
	return []FieldSchema{
		{
			Name:    "targetId",
			GoName:  "TargetID",
			GoType:  "string",
			JSONTag: `json:"targetId,omitempty"`,
			SchemaJSON: json.RawMessage(
				`{"type":"string","description":"Exact target id; mutually exclusive with urlPattern."}`,
			),
		},
		{
			Name:    "urlPattern",
			GoName:  "URLPattern",
			GoType:  "string",
			JSONTag: `json:"urlPattern,omitempty"`,
			SchemaJSON: json.RawMessage(
				`{"type":"string","description":"Substring of the target URL; mutually exclusive with targetId."}`,
			),
		},
	}
}

// marshalDeterministic JSON-marshals a map with stable key ordering so the
// generated output byte-equals on every run.
func marshalDeterministic(v map[string]any) (json.RawMessage, error) {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(strconv.Quote(k))
		buf.WriteByte(':')
		sub, err := marshalAnyDeterministic(v[k])
		if err != nil {
			return nil, err
		}
		buf.Write(sub)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func marshalAnyDeterministic(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		return marshalDeterministic(t)
	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			sub, err := marshalAnyDeterministic(e)
			if err != nil {
				return nil, err
			}
			buf.Write(sub)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	default:
		return json.Marshal(v)
	}
}

// goName converts a CDP camelCase name to an exported Go identifier. Special
// cases for common acronyms (URL, ID) match the existing hand-written code.
func goName(s string) string {
	if s == "" {
		return ""
	}
	out := strings.ToUpper(s[:1]) + s[1:]
	for _, ac := range []string{"Url", "Id", "Json", "Http", "Css", "Dom", "Cpu", "Gpu", "Ip"} {
		// Only replace at the end of the identifier or before another uppercase
		// boundary, to avoid corrupting names like "Idea" or "Curlytown".
		if strings.HasSuffix(out, ac) {
			out = strings.TrimSuffix(out, ac) + strings.ToUpper(ac)
		}
	}
	return out
}
