package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// Tool is one capability the model can invoke. Protean's harness ships with
// none: the tool set is assembled at boot (S6) and shaped further per org and
// per project, which is what "bare-bones core, extensible surface" means.
//
// Execute returns the string the model will read. A tool reports a failure as
// an error; the loop turns it into a tool result the model can react to, so a
// failing tool never ends a conversation. Execute must respect ctx.
type Tool interface {
	Name() string
	Def() ports.ToolDef
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// Registry rejections. Match with errors.Is.
var (
	ErrToolNotFound  = errors.New("harness: tool not found")
	ErrDuplicateTool = errors.New("harness: tool already registered")
	ErrInvalidTool   = errors.New("harness: invalid tool")
)

// Registry holds the tools available to a run. It is safe for concurrent use,
// so a tool set can be extended while the process serves.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	order []string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// Register adds a tool. A tool whose Name disagrees with its Def, or whose
// name is already taken, is rejected: the model addresses tools by name, so an
// ambiguous name is a defect, not a preference.
func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("%w: nil", ErrInvalidTool)
	}
	name := t.Name()
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidTool)
	}
	if def := t.Def(); def.Name != name {
		return fmt.Errorf("%w: %q advertises itself as %q", ErrInvalidTool, name, def.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.tools[name]; dup {
		return fmt.Errorf("%w: %q", ErrDuplicateTool, name)
	}
	r.tools[name] = t
	r.order = append(r.order, name)
	return nil
}

// MustRegister is Register for boot wiring, where a duplicate is a programming
// error.
func (r *Registry) MustRegister(t Tool) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

// Lookup returns the tool registered under name.
func (r *Registry) Lookup(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrToolNotFound, name)
	}
	return t, nil
}

// Defs returns the tool definitions in registration order, which is the order
// the model sees them in.
func (r *Registry) Defs() []ports.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]ports.ToolDef, 0, len(r.order))
	for _, name := range r.order {
		defs = append(defs, r.tools[name].Def())
	}
	return defs
}

// Names returns the registered tool names in registration order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string(nil), r.order...)
}

// Len is how many tools are registered.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.order)
}

// FuncTool is a Tool built from a definition and a function, for tools with no
// state of their own.
type FuncTool struct {
	def ports.ToolDef
	fn  func(ctx context.Context, args json.RawMessage) (string, error)
}

var _ Tool = (*FuncTool)(nil)

// NewFuncTool wraps fn as a Tool.
func NewFuncTool(def ports.ToolDef, fn func(ctx context.Context, args json.RawMessage) (string, error)) *FuncTool {
	return &FuncTool{def: def, fn: fn}
}

func (t *FuncTool) Name() string { return t.def.Name }

func (t *FuncTool) Def() ports.ToolDef { return t.def }

func (t *FuncTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.fn(ctx, args)
}
