package layout_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
)

const (
	org     = "01ORG"
	project = "01PROJ"
	thread  = "01THREAD"
)

func TestPaths(t *testing.T) {
	const data = "/data"

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"orgs root", layout.OrgsRoot(data), "/data/protean-organizations"},
		{"org dir", layout.OrgDir(data, org), "/data/protean-organizations/01ORG"},
		{"org control", layout.OrgProteanDir(data, org), "/data/protean-organizations/01ORG/.protean"},
		{"org configs", layout.OrgConfigsPath(data, org), "/data/protean-organizations/01ORG/.protean/configs.json"},
		{"org agents", layout.OrgAgentsMD(data, org), "/data/protean-organizations/01ORG/AGENTS.md"},
		{"user memory", layout.UserMemoryPath(data, org, "01USER"), "/data/protean-organizations/01ORG/.protean/memories/users/01USER/MEMORY.md"},
		{"project memory", layout.ProjectMemoryPath(data, org, project), "/data/protean-organizations/01ORG/.protean/memories/projects/01PROJ/MEMORY.md"},
		{"skill", layout.SkillPath(data, org, "pdf-tools"), "/data/protean-organizations/01ORG/.protean/skills/pdf-tools/SKILL.md"},
		{"projects dir", layout.ProjectsDir(data, org), "/data/protean-organizations/01ORG/projects"},
		{"project dir", layout.ProjectDir(data, org, project), "/data/protean-organizations/01ORG/projects/01PROJ"},
		{"project control", layout.ProjectProteanDir(data, org, project), "/data/protean-organizations/01ORG/projects/01PROJ/.protean"},
		{"project configs", layout.ProjectConfigsPath(data, org, project), "/data/protean-organizations/01ORG/projects/01PROJ/.protean/configs.json"},
		{"project agents", layout.ProjectAgentsMD(data, org, project), "/data/protean-organizations/01ORG/projects/01PROJ/AGENTS.md"},
		{"threads dir", layout.ThreadsDir(data, org, project), "/data/protean-organizations/01ORG/projects/01PROJ/.protean/threads"},
		{"thread dir", layout.ThreadDir(data, org, project, thread), "/data/protean-organizations/01ORG/projects/01PROJ/.protean/threads/01THREAD"},
		{"thread json", layout.ThreadJSONPath(data, org, project, thread), "/data/protean-organizations/01ORG/projects/01PROJ/.protean/threads/01THREAD/thread.json"},
		{"messages dir", layout.MessagesDir(data, org, project, thread), "/data/protean-organizations/01ORG/projects/01PROJ/.protean/threads/01THREAD/messages"},
		{"message", layout.MessagePath(data, org, project, thread, "01MSG"), "/data/protean-organizations/01ORG/projects/01PROJ/.protean/threads/01THREAD/messages/01MSG.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestValidID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"01J8ZK3XQ7V2H4N6P8R0S2T4W6", true},
		{"org-1", true},
		{"pdf.tools_v2", true},
		{"", false},
		{".", false},
		{"..", false},
		{".hidden", false},
		{"a/b", false},
		{"../escape", false},
		{"has space", false},
		{"semi;colon", false},
		{"null\x00byte", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := layout.ValidID(tt.id); got != tt.want {
				t.Errorf("ValidID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestCheckIDRejection(t *testing.T) {
	err := layout.CheckID("org id", "../escape")
	if !errors.Is(err, layout.ErrInvalidID) {
		t.Fatalf("got %v, want ErrInvalidID", err)
	}
}

func TestMessageIDFromFile(t *testing.T) {
	tests := []struct {
		name   string
		wantID string
		wantOK bool
	}{
		{"01MSG.json", "01MSG", true},
		{"01MSG.txt", "", false},
		{"01MSG", "", false},
		{".tmp-1234.json", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := layout.MessageIDFromFile(tt.name)
			if id != tt.wantID || ok != tt.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestEnsureProjectCreatesSkeleton(t *testing.T) {
	data := t.TempDir()

	if err := layout.EnsureProject(data, org, project); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	wantDirs := []string{
		layout.OrgProteanDir(data, org),
		filepath.Join(layout.MemoriesDir(data, org), layout.UserMemoriesDir),
		filepath.Join(layout.MemoriesDir(data, org), layout.ProjectMemoriesIn),
		layout.SkillsDir(data, org),
		layout.ProjectDir(data, org, project),
		layout.ThreadsDir(data, org, project),
	}
	for _, dir := range wantDirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("stat %q: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q is not a directory", dir)
		}
	}

	for _, f := range []string{layout.OrgAgentsMD(data, org), layout.ProjectAgentsMD(data, org, project)} {
		info, err := os.Stat(f)
		if err != nil {
			t.Errorf("stat %q: %v", f, err)
			continue
		}
		if info.Size() != 0 {
			t.Errorf("%q should start empty, got %d bytes", f, info.Size())
		}
	}
}

func TestEnsureProjectIsIdempotentAndKeepsContent(t *testing.T) {
	data := t.TempDir()

	if err := layout.EnsureProject(data, org, project); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	agents := layout.ProjectAgentsMD(data, org, project)
	if err := os.WriteFile(agents, []byte("be helpful\n"), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := layout.EnsureProject(data, org, project); err != nil {
		t.Fatalf("EnsureProject again: %v", err)
	}

	got, err := os.ReadFile(agents)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(got) != "be helpful\n" {
		t.Errorf("AGENTS.md was clobbered: %q", got)
	}
}

func TestEnsureRejectsBadIDs(t *testing.T) {
	data := t.TempDir()

	if err := layout.EnsureOrg(data, "../escape"); !errors.Is(err, layout.ErrInvalidID) {
		t.Errorf("EnsureOrg: got %v, want ErrInvalidID", err)
	}
	if err := layout.EnsureProject(data, org, ".."); !errors.Is(err, layout.ErrInvalidID) {
		t.Errorf("EnsureProject: got %v, want ErrInvalidID", err)
	}
}
