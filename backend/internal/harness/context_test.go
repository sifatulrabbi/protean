package harness

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
)

func newContextRig(t *testing.T) (*ContextBuilder, string) {
	t.Helper()
	dataDir := t.TempDir()
	if err := layout.EnsureProject(dataDir, testOrg, testProject); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	return NewContextBuilder(dataDir, discardLogger()), dataDir
}

func defaultInput() ContextInput {
	return ContextInput{OrgID: testOrg, ProjectID: testProject, UserID: testUser}
}

func TestSystemPromptIsEmptyWithoutAnyFiles(t *testing.T) {
	b, _ := newContextRig(t)
	if got := b.SystemPrompt(defaultInput()); got != "" {
		t.Fatalf("SystemPrompt = %q, want empty", got)
	}
}

func TestSystemPromptConcatenatesAgentsDocs(t *testing.T) {
	b, dataDir := newContextRig(t)
	writeFile(t, layout.OrgAgentsMD(dataDir, testOrg), "ORG RULES\n")
	writeFile(t, layout.ProjectAgentsMD(dataDir, testOrg, testProject), "PROJECT RULES\n")

	got := b.SystemPrompt(defaultInput())
	if got != "ORG RULES\n\nPROJECT RULES" {
		t.Fatalf("SystemPrompt = %q", got)
	}
}

func TestSystemPromptSkipsMissingAndEmptyFiles(t *testing.T) {
	tests := []struct {
		name    string
		org     string
		project string
		want    string
	}{
		{"only org", "ORG RULES", "", "ORG RULES"},
		{"only project", "", "PROJECT RULES", "PROJECT RULES"},
		{"whitespace only is empty", "   \n\n", "PROJECT RULES", "PROJECT RULES"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, dataDir := newContextRig(t)
			if tc.org != "" {
				writeFile(t, layout.OrgAgentsMD(dataDir, testOrg), tc.org)
			}
			if tc.project != "" {
				writeFile(t, layout.ProjectAgentsMD(dataDir, testOrg, testProject), tc.project)
			}
			if got := b.SystemPrompt(defaultInput()); got != tc.want {
				t.Fatalf("SystemPrompt = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSystemPromptMemories(t *testing.T) {
	b, dataDir := newContextRig(t)
	writeFile(t, layout.ProjectMemoryPath(dataDir, testOrg, testProject), "the API lives in cmd/")
	writeFile(t, layout.UserMemoryPath(dataDir, testOrg, testUser), "prefers short answers")

	got := b.SystemPrompt(defaultInput())
	if !strings.Contains(got, projectMemHead) || !strings.Contains(got, "the API lives in cmd/") {
		t.Errorf("project memory missing from %q", got)
	}
	if !strings.Contains(got, userMemHead) || !strings.Contains(got, "prefers short answers") {
		t.Errorf("user memory missing from %q", got)
	}
	if strings.Index(got, projectMemHead) > strings.Index(got, userMemHead) {
		t.Error("the project memory must come before the user memory")
	}
}

func TestSystemPromptLoadsOnlyTheRunningUsersMemory(t *testing.T) {
	b, dataDir := newContextRig(t)
	writeFile(t, layout.UserMemoryPath(dataDir, testOrg, testUser), "mine")
	writeFile(t, layout.UserMemoryPath(dataDir, testOrg, "user-2"), "someone else's")

	got := b.SystemPrompt(defaultInput())
	if !strings.Contains(got, "mine") {
		t.Error("the running user's memory was not loaded")
	}
	if strings.Contains(got, "someone else's") {
		t.Error("another user's memory leaked into the prompt")
	}

	anonymous := b.SystemPrompt(ContextInput{OrgID: testOrg, ProjectID: testProject})
	if strings.Contains(anonymous, "mine") {
		t.Error("a run without a user id loaded a user memory")
	}
}

func TestSystemPromptRendersTasks(t *testing.T) {
	b, _ := newContextRig(t)
	in := defaultInput()
	in.Tasks = []ports.Task{
		{ID: "t1", Title: "Read the spec", Status: ports.TaskDone},
		{ID: "t2", Title: "Write the loop", Status: ports.TaskInProgress},
		{ID: "t3", Title: "Test it", Status: ports.TaskTodo},
	}

	got := b.SystemPrompt(in)
	for _, want := range []string{
		tasksHeading,
		"- [x] t1: Read the spec",
		"- [~] t2: Write the loop",
		"- [ ] t3: Test it",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt is missing %q:\n%s", want, got)
		}
	}
}

func TestSystemPromptOmitsAnEmptyTaskList(t *testing.T) {
	b, _ := newContextRig(t)
	if got := b.SystemPrompt(defaultInput()); strings.Contains(got, tasksHeading) {
		t.Fatalf("an empty task list produced a tasks section: %q", got)
	}
}

func writeSkill(t *testing.T, dataDir, dir, body string) {
	t.Helper()
	writeFile(t, layout.SkillPath(dataDir, testOrg, dir), body)
}

func TestSystemPromptSkillCatalog(t *testing.T) {
	b, dataDir := newContextRig(t)

	writeSkill(t, dataDir, "pdf-forms", "---\nname: pdf-forms\ndescription: Fill in PDF forms.\n---\n\nBody.\n")
	writeSkill(t, dataDir, "quoted", "---\nname: \"quoted-skill\"\ndescription: 'Handles quotes.'\n---\n")
	writeSkill(t, dataDir, "folded", "---\nname: folded\ndescription: |\n  A long description\n  over two lines.\nlicense: MIT\n---\n")
	writeSkill(t, dataDir, "nameless", "---\ndescription: No name declared.\n---\n")

	got := b.SystemPrompt(defaultInput())
	for _, want := range []string{
		skillsHeading,
		"- pdf-forms: Fill in PDF forms.",
		"- quoted-skill: Handles quotes.",
		"- folded: A long description over two lines.",
		"- nameless: No name declared.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Body.") {
		t.Error("a skill body was loaded into the prompt; only the metadata belongs there")
	}
}

func TestSystemPromptSkipsUnusableSkills(t *testing.T) {
	var logs bytes.Buffer
	dataDir := t.TempDir()
	if err := layout.EnsureProject(dataDir, testOrg, testProject); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	b := NewContextBuilder(dataDir, slog.New(slog.NewTextHandler(&logs, nil)))

	writeSkill(t, dataDir, "good", "---\nname: good\ndescription: Works.\n---\n")
	writeSkill(t, dataDir, "unterminated", "---\nname: broken\ndescription: never closed\n")
	writeSkill(t, dataDir, "no-frontmatter", "# Just a document\n\nNo frontmatter here.\n")
	// A directory without a SKILL.md is not a skill at all.
	if err := os.MkdirAll(layout.SkillDir(dataDir, testOrg, "empty-dir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got := b.SystemPrompt(defaultInput())
	if !strings.Contains(got, "- good: Works.") {
		t.Errorf("the usable skill is missing:\n%s", got)
	}
	for _, unwanted := range []string{"broken", "no-frontmatter", "empty-dir"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unusable skill %q reached the prompt:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(logs.String(), "skipping skill with unreadable frontmatter") {
		t.Errorf("the skipped skills were not logged:\n%s", logs.String())
	}
}

func TestSystemPromptSectionOrder(t *testing.T) {
	b, dataDir := newContextRig(t)
	writeFile(t, layout.OrgAgentsMD(dataDir, testOrg), "ORG RULES")
	writeFile(t, layout.ProjectMemoryPath(dataDir, testOrg, testProject), "a memory")
	writeSkill(t, dataDir, "a-skill", "---\nname: a-skill\ndescription: Does things.\n---\n")

	in := defaultInput()
	in.Tasks = []ports.Task{{ID: "t1", Title: "Do it", Status: ports.TaskTodo}}
	got := b.SystemPrompt(in)

	order := []string{"ORG RULES", memoryHeading, tasksHeading, skillsHeading}
	last := -1
	for _, section := range order {
		i := strings.Index(got, section)
		if i < 0 {
			t.Fatalf("section %q is missing:\n%s", section, got)
		}
		if i < last {
			t.Fatalf("section %q is out of order:\n%s", section, got)
		}
		last = i
	}
}

func TestSystemPromptSkipsContextSymlinksOutsideTheirRoots(t *testing.T) {
	tests := []struct {
		name string
		link func(dataDir string) string
		body string
	}{
		{
			name: "AGENTS.md",
			link: func(dataDir string) string { return layout.OrgAgentsMD(dataDir, testOrg) },
			body: "ESCAPED AGENT RULES",
		},
		{
			name: "memory",
			link: func(dataDir string) string { return layout.ProjectMemoryPath(dataDir, testOrg, testProject) },
			body: "ESCAPED MEMORY",
		},
		{
			name: "SKILL.md",
			link: func(dataDir string) string { return layout.SkillPath(dataDir, testOrg, "escaped-skill") },
			body: "---\nname: escaped-skill\ndescription: ESCAPED SKILL\n---\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			dataDir := t.TempDir()
			if err := layout.EnsureProject(dataDir, testOrg, testProject); err != nil {
				t.Fatalf("EnsureProject: %v", err)
			}
			outside := t.TempDir() + "/outside.md"
			writeFile(t, outside, tc.body)
			link := tc.link(dataDir)
			if err := os.MkdirAll(dirOf(link), 0o755); err != nil {
				t.Fatalf("mkdir link parent: %v", err)
			}
			if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
				t.Fatalf("remove existing context file: %v", err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatalf("symlink: %v", err)
			}

			b := NewContextBuilder(dataDir, slog.New(slog.NewTextHandler(&logs, nil)))
			got := b.SystemPrompt(defaultInput())
			if strings.Contains(got, "ESCAPED") {
				t.Fatalf("escaped file reached system prompt: %q", got)
			}
			if !strings.Contains(logs.String(), "escapes root") {
				t.Fatalf("escape was not logged: %s", logs.String())
			}
		})
	}
}

func TestParseSkillFrontmatter(t *testing.T) {
	tests := []struct {
		name            string
		body            string
		wantName        string
		wantDescription string
		wantOK          bool
	}{
		{"plain", "---\nname: a\ndescription: b\n---\n", "a", "b", true},
		{"crlf", "---\r\nname: a\r\ndescription: b\r\n---\r\n", "a", "b", true},
		{"extra keys ignored", "---\nname: a\nversion: 2\ndescription: b\n---\n", "a", "b", true},
		{"nested keys ignored", "---\nname: a\nmeta:\n  owner: x\ndescription: b\n---\n", "a", "b", true},
		{"comments ignored", "---\n# a comment\nname: a\n---\n", "a", "", true},
		{"colon in the value", "---\nname: a\ndescription: use it: carefully\n---\n", "a", "use it: carefully", true},
		{"block scalar", "---\ndescription: >\n  one\n  two\nname: a\n---\n", "a", "one two", true},
		{"empty frontmatter", "---\n---\n", "", "", true},
		// An editor's leading blank line or byte-order mark is not the author's
		// mistake, so neither costs them the skill.
		{"leading blank line", "\n\n---\nname: a\n---\n", "a", "", true},
		{"byte order mark", "\ufeff---\nname: a\n---\n", "a", "", true},
		{"no frontmatter", "# doc\n", "", "", false},
		{"unterminated", "---\nname: a\n", "", "", false},
		{"fence after prose", "# doc\n---\nname: a\n---\n", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, description, ok := parseSkillFrontmatter([]byte(tc.body))
			if ok != tc.wantOK || name != tc.wantName || description != tc.wantDescription {
				t.Fatalf("parseSkillFrontmatter = (%q, %q, %v), want (%q, %q, %v)",
					name, description, ok, tc.wantName, tc.wantDescription, tc.wantOK)
			}
		})
	}
}

func TestListSkillsFallsBackToTheDirectoryName(t *testing.T) {
	b, dataDir := newContextRig(t)
	writeSkill(t, dataDir, "on-disk-name", "---\ndescription: Only a description.\n---\n")

	skills := b.listSkills(testOrg)
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "on-disk-name" || skills[0].Dir != "on-disk-name" {
		t.Fatalf("skill = %+v", skills[0])
	}
}

func TestListSkillsWithoutASkillsDirectory(t *testing.T) {
	b := NewContextBuilder(t.TempDir(), discardLogger())
	if skills := b.listSkills(testOrg); skills != nil {
		t.Fatalf("listSkills = %+v, want nothing", skills)
	}
}
