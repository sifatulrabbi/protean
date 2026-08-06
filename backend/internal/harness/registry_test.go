package harness

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// mismatchedTool advertises a different name than it answers to.
type mismatchedTool struct{ echoTool }

func (t *mismatchedTool) Def() ports.ToolDef { return ports.ToolDef{Name: "something-else"} }

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	grep := &echoTool{name: "Grep", result: "ok"}
	list := &echoTool{name: "ListDir", result: "ok"}

	if err := r.Register(grep); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(list); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Lookup("Grep")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != Tool(grep) {
		t.Error("Lookup returned a different tool")
	}
	if r.Len() != 2 {
		t.Errorf("Len = %d, want 2", r.Len())
	}

	// Registration order is what the model sees.
	names := r.Names()
	if len(names) != 2 || names[0] != "Grep" || names[1] != "ListDir" {
		t.Errorf("Names = %v", names)
	}
	defs := r.Defs()
	if len(defs) != 2 || defs[0].Name != "Grep" || defs[1].Name != "ListDir" {
		t.Errorf("Defs = %+v", defs)
	}
}

func TestRegistryRejections(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&echoTool{name: "Grep"})

	tests := []struct {
		name string
		tool Tool
		want error
	}{
		{"nil", nil, ErrInvalidTool},
		{"no name", &echoTool{}, ErrInvalidTool},
		{"name disagrees with definition", &mismatchedTool{echoTool{name: "Grep2"}}, ErrInvalidTool},
		{"duplicate", &echoTool{name: "Grep"}, ErrDuplicateTool},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := r.Register(tc.tool); !errors.Is(err, tc.want) {
				t.Fatalf("Register = %v, want %v", err, tc.want)
			}
		})
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d, want the rejected tools to have been left out", r.Len())
	}
}

func TestRegistryLookupUnknown(t *testing.T) {
	if _, err := NewRegistry().Lookup("Nope"); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("Lookup = %v, want ErrToolNotFound", err)
	}
}

func TestRegistryShipsEmpty(t *testing.T) {
	// Protean's harness has no built-in tools: the tool set is assembled at
	// boot and shaped per org.
	r := NewRegistry()
	if r.Len() != 0 || len(r.Defs()) != 0 {
		t.Fatalf("a new registry is not empty: %v", r.Names())
	}
}

func TestFuncTool(t *testing.T) {
	def := ports.ToolDef{Name: "Echo", Description: "echoes", Parameters: json.RawMessage(`{"type":"object"}`)}
	tool := NewFuncTool(def, func(_ context.Context, args json.RawMessage) (string, error) {
		return string(args), nil
	})

	if tool.Name() != "Echo" || tool.Def().Description != "echoes" {
		t.Fatalf("tool identity = %+v", tool.Def())
	}
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"a":1}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != `{"a":1}` {
		t.Errorf("Execute = %q", out)
	}

	r := NewRegistry()
	if err := r.Register(tool); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestMustRegisterPanicsOnADuplicate(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister did not panic on a duplicate")
		}
	}()
	r := NewRegistry()
	r.MustRegister(&echoTool{name: "Grep"})
	r.MustRegister(&echoTool{name: "Grep"})
}

func TestRegistryIsConcurrencySafe(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&echoTool{name: "Grep"})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			r.Defs()
			r.Names()
			r.Len()
		}
	}()
	for i := 0; i < 100; i++ {
		_, _ = r.Lookup("Grep")
	}
	<-done
}
