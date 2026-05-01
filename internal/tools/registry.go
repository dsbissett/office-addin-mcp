package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// Tool is a single registered tool. Run is invoked by Dispatch after schema
// validation; the params have already been validated against Schema.
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Run         func(ctx context.Context, params json.RawMessage, env *RunEnv) Result

	// NoSession marks lifecycle tools (addin.detect, addin.launch, addin.stop)
	// that do not need a live CDP connection. The dispatcher skips session
	// acquisition for these, so they run even when no WebView2 is available
	// yet — which is the whole point of addin.launch.
	NoSession bool

	compiled *jsonschema.Schema
}

// Registry holds the active tool set. It is safe for concurrent reads after
// MustRegister calls have completed during init/wireup.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]*Tool{}}
}

// Register adds a tool. Returns an error if the name is duplicate or the
// schema does not compile.
func (r *Registry) Register(t Tool) error {
	if t.Name == "" {
		return fmt.Errorf("tools.Register: empty Name")
	}
	if t.Run == nil {
		return fmt.Errorf("tools.Register %q: nil Run", t.Name)
	}
	compiled, err := compileSchema(t.Name, t.Schema)
	if err != nil {
		return fmt.Errorf("tools.Register %q: schema: %w", t.Name, err)
	}
	t.compiled = compiled

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.tools[t.Name]; dup {
		return fmt.Errorf("tools.Register %q: already registered", t.Name)
	}
	r.tools[t.Name] = &t
	return nil
}

// MustRegister wraps Register and panics on failure. Suitable for static
// wireup at process start.
func (r *Registry) MustRegister(t Tool) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

// Get returns the tool with the given name, or (nil, false).
func (r *Registry) Get(name string) (*Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns the registered tools sorted by name.
func (r *Registry) List() []*Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
