package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
)

// Context assembly turns the org's files into the system prompt.
//
// The harness has no built-in persona: everything the model is told about how
// to behave comes from files on disk, which is what makes one harness serve
// every org. The sections, in order:
//
//  1. The behavior documents, verbatim: org AGENTS.md then project AGENTS.md.
//     The project file appends to the org file, it never replaces it. Missing
//     and empty files are skipped. S10 owns the real merge semantics — config
//     merging, validation, includes — and will replace readAgents.
//  2. Memories: the project memory, then this user's memory.
//  3. The thread's task list, compactly rendered.
//  4. The names and descriptions of the org's installed skills. Bodies are not
//     loaded here: S6's ReadSkill pulls one in when the model asks for it.
//
// A section with nothing in it is omitted, and an org with no files at all
// produces an empty prompt — in which case no system message is sent at all.
const (
	memoryHeading  = "# Memory"
	projectMemHead = "## Project memory"
	userMemHead    = "## Your memory for this user"
	tasksHeading   = "# Tasks"
	skillsHeading  = "# Skills"

	skillsPreamble = "The following skills are installed. Only their descriptions are shown; load a skill before you rely on it."
	tasksPreamble  = "The current task list for this thread. Keep it up to date as you work."
)

// ContextInput is what the system prompt is assembled from, beyond the files
// on disk. UserID scopes the user memory and comes from the caller, because
// only the caller knows who is talking.
type ContextInput struct {
	OrgID     string
	ProjectID string
	UserID    string
	Tasks     []ports.Task
}

// ContextBuilder assembles system prompts from one data directory.
type ContextBuilder struct {
	dataDir string
	log     *slog.Logger
}

// NewContextBuilder returns a builder rooted at dataDir.
func NewContextBuilder(dataDir string, logger *slog.Logger) *ContextBuilder {
	if logger == nil {
		logger = slog.Default()
	}
	return &ContextBuilder{dataDir: dataDir, log: logger.With("component", "harness.context")}
}

// SystemPrompt renders the prompt for one invocation. It never fails: a file
// that cannot be read is left out and logged, because a broken memory file
// must not take the whole agent down.
func (b *ContextBuilder) SystemPrompt(in ContextInput) string {
	var sections []string

	if s := b.agentsDocs(in); s != "" {
		sections = append(sections, s)
	}
	if s := b.memories(in); s != "" {
		sections = append(sections, s)
	}
	if s := renderTasks(in.Tasks); s != "" {
		sections = append(sections, s)
	}
	if s := b.skillCatalog(in.OrgID); s != "" {
		sections = append(sections, s)
	}

	return strings.Join(sections, "\n\n")
}

// agentsDocs concatenates the org and project behavior documents.
func (b *ContextBuilder) agentsDocs(in ContextInput) string {
	var parts []string
	for _, file := range []struct {
		root string
		path string
	}{
		{layout.OrgDir(b.dataDir, in.OrgID), layout.OrgAgentsMD(b.dataDir, in.OrgID)},
		{layout.ProjectDir(b.dataDir, in.OrgID, in.ProjectID), layout.ProjectAgentsMD(b.dataDir, in.OrgID, in.ProjectID)},
	} {
		if text := b.readFile(file.root, file.path); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// memories renders the project and user memory files.
func (b *ContextBuilder) memories(in ContextInput) string {
	var parts []string
	if text := b.readFile(
		layout.ProjectMemoryDir(b.dataDir, in.OrgID, in.ProjectID),
		layout.ProjectMemoryPath(b.dataDir, in.OrgID, in.ProjectID),
	); text != "" {
		parts = append(parts, projectMemHead+"\n\n"+text)
	}
	if in.UserID != "" {
		if text := b.readFile(
			layout.UserMemoryDir(b.dataDir, in.OrgID, in.UserID),
			layout.UserMemoryPath(b.dataDir, in.OrgID, in.UserID),
		); text != "" {
			parts = append(parts, userMemHead+"\n\n"+text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return memoryHeading + "\n\n" + strings.Join(parts, "\n\n")
}

// renderTasks renders the thread's task list as a checklist.
func renderTasks(tasks []ports.Task) string {
	if len(tasks) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(tasksHeading)
	sb.WriteString("\n\n")
	sb.WriteString(tasksPreamble)
	sb.WriteString("\n")
	for _, t := range tasks {
		sb.WriteString("\n- [")
		switch t.Status {
		case ports.TaskDone:
			sb.WriteString("x")
		case ports.TaskInProgress:
			sb.WriteString("~")
		default:
			sb.WriteString(" ")
		}
		sb.WriteString("] ")
		sb.WriteString(t.ID)
		sb.WriteString(": ")
		sb.WriteString(t.Title)
	}
	return sb.String()
}

// skillCatalog lists the org's installed skills by name and description.
func (b *ContextBuilder) skillCatalog(orgID string) string {
	skills := b.listSkills(orgID)
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(skillsHeading)
	sb.WriteString("\n\n")
	sb.WriteString(skillsPreamble)
	sb.WriteString("\n")
	for _, s := range skills {
		sb.WriteString("\n- ")
		sb.WriteString(s.Name)
		if s.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(s.Description)
		}
	}
	return sb.String()
}

// SkillMeta is one installed skill's advertised identity.
type SkillMeta struct {
	// Dir is the directory name under the org's skills root, which is what
	// ReadSkill (S6) will address the skill by.
	Dir         string
	Name        string
	Description string
}

// listSkills reads the frontmatter of every installed SKILL.md, in directory
// order (which is lexical). Enablement — the org and project configs deciding
// which skills apply — is S9's; every installed skill is listed for now.
func (b *ContextBuilder) listSkills(orgID string) []SkillMeta {
	root := layout.SkillsDir(b.dataDir, orgID)
	resolvedRoot, err := b.containedPath(layout.OrgDir(b.dataDir, orgID), root)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			b.log.Warn("cannot list skills", "org_id", orgID, "path", root, "err", err)
		}
		return nil
	}
	entries, err := os.ReadDir(resolvedRoot)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			b.log.Warn("cannot list skills", "org_id", orgID, "path", root, "err", err)
		}
		return nil
	}

	skills := make([]SkillMeta, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || !layout.ValidID(e.Name()) {
			continue
		}
		path := layout.SkillPath(b.dataDir, orgID, e.Name())
		data, err := b.readFileBytes(layout.SkillDir(b.dataDir, orgID, e.Name()), path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				b.log.Warn("cannot read skill", "org_id", orgID, "skill", e.Name(), "err", err)
			}
			continue
		}
		name, description, ok := parseSkillFrontmatter(data)
		if !ok {
			// A skill that does not declare itself cannot be described to the
			// model, and guessing would put a wrong description in front of it.
			b.log.Warn("skipping skill with unreadable frontmatter", "org_id", orgID, "skill", e.Name(), "path", path)
			continue
		}
		if name == "" {
			name = e.Name()
		}
		skills = append(skills, SkillMeta{Dir: e.Name(), Name: name, Description: description})
	}
	return skills
}

// parseSkillFrontmatter reads the YAML frontmatter of a SKILL.md leniently.
//
// It is not a YAML parser and does not want to be: the agentskills.io format
// puts `name` and `description` at the top level of a `---` fenced block, and
// those two scalars are all the catalog needs. Plain, quoted, and block
// scalars are understood; anything else in the block is ignored. It reports
// false when there is no terminated frontmatter block at all.
func parseSkillFrontmatter(data []byte) (name, description string, ok bool) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	// An editor's byte-order mark and a leading blank line are not the skill
	// author's mistake, and refusing the file over either would report the
	// wrong problem.
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.TrimLeft(text, " \t\n")
	rest, found := strings.CutPrefix(text, "---\n")
	if !found {
		return "", "", false
	}

	lines := strings.Split(rest, "\n")
	end := -1
	for i, line := range lines {
		if strings.TrimRight(line, " \t") == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return "", "", false
	}
	lines = lines[:end]

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		// Only top-level keys matter; an indented line belongs to a nested
		// structure this parser does not model.
		if line != strings.TrimLeft(line, " \t") {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key != "name" && key != "description" {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "|" || value == ">" || value == "|-" || value == ">-" {
			value, i = readBlockScalar(lines, i+1)
		} else {
			value = unquote(value)
		}
		switch key {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, description, true
}

// readBlockScalar joins the indented lines that follow a `|` or `>` marker and
// returns the index of the last line it consumed.
func readBlockScalar(lines []string, start int) (string, int) {
	var parts []string
	i := start
	for ; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line == strings.TrimLeft(line, " \t") {
			break
		}
		parts = append(parts, strings.TrimSpace(line))
	}
	return strings.Join(parts, " "), i - 1
}

// unquote strips one layer of matching quotes.
func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// readFile returns a trimmed file body, or "" when it is missing, unreadable,
// or resolves outside root.
func (b *ContextBuilder) readFile(root, path string) string {
	data, err := b.readFileBytes(root, path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			b.log.Warn("cannot read context file", "path", path, "err", err)
		}
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (b *ContextBuilder) readFileBytes(root, path string) ([]byte, error) {
	resolved, err := b.containedPath(root, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

// containedPath resolves symlinks in both root and path, then proves the file
// remains below the intended tenant scope before it is opened.
func (b *ContextBuilder) containedPath(root, path string) (string, error) {
	resolvedDataDir, err := filepath.EvalSymlinks(b.dataDir)
	if err != nil {
		return "", err
	}
	absDataDir, err := filepath.Abs(b.dataDir)
	if err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootRel, err := filepath.Rel(absDataDir, absRoot)
	if err != nil || rootRel == ".." || strings.HasPrefix(rootRel, ".."+string(filepath.Separator)) || filepath.IsAbs(rootRel) {
		return "", fmt.Errorf("context root %q escapes data directory %q", root, b.dataDir)
	}
	expectedRoot := filepath.Clean(filepath.Join(resolvedDataDir, rootRel))
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if resolvedRoot != expectedRoot {
		return "", fmt.Errorf("context root %q escapes its tenant scope", root)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("context path %q escapes root %q", path, root)
	}
	return resolvedPath, nil
}
