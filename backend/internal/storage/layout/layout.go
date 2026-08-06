// Package layout is the single authority on the host-side storage layout. Every
// path under the data directory is built here; no other package may join
// directory names of its own.
//
// The tree, rooted at <dataDir>:
//
//	protean-organizations/<org-ulid>/
//	  .protean/
//	    configs.json
//	    memories/users/<user-ulid>/MEMORY.md
//	    memories/projects/<project-ulid>/MEMORY.md
//	    skills/<skill-name>/SKILL.md
//	  AGENTS.md
//	  projects/<project-ulid>/
//	    .protean/
//	      configs.json
//	      threads/<thread-ulid>/
//	        thread.json
//	        messages/<message-ulid>.json
//	    AGENTS.md
//	    ...user + agent files...
//
// IDs are ULIDs, so lexical order equals creation order and nothing sorts by
// timestamp.
//
// The path builders do not validate their arguments: they are pure joins, and
// callers that accept an ID from outside the process must run it through
// ValidID first. EnsureOrg and EnsureProject do validate, because they touch
// the filesystem.
package layout

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// OrgsDirName holds one subdirectory per organization.
	OrgsDirName = "protean-organizations"

	// ProjectsDirName holds one subdirectory per project inside an org tree.
	ProjectsDirName = "projects"

	// ControlDirName is Protean's control directory. It exists at the org root
	// and at every project root. Inside a sandbox it is masked out: the agent
	// never reaches it.
	ControlDirName = ".protean"

	// AgentsFileName is the behavior document. The org-level file always
	// applies; the project-level one concatenates after it.
	AgentsFileName = "AGENTS.md"

	// ConfigsFileName is the admin-tweakable config. A project's file merges
	// over the org's, field by field.
	ConfigsFileName = "configs.json"

	MemoriesDirName   = "memories"
	UserMemoriesDir   = "users"
	ProjectMemoriesIn = "projects"
	MemoryFileName    = "MEMORY.md"

	SkillsDirName    = "skills"
	SkillFileName    = "SKILL.md"
	ThreadsDirName   = "threads"
	ThreadFileName   = "thread.json"
	MessagesDirName  = "messages"
	MessageExtension = ".json"
)

// Directory and file modes. Control directories are owner-only because only the
// harness reads them; project directories are group/world readable so the
// container runtime can bind-mount them.
const (
	DirMode        = 0o755
	ControlDirMode = 0o700
	FileMode       = 0o644
)

// idPattern is what an on-disk identifier may look like. ULIDs pass, and so do
// the hyphenated names used for skills and in tests. Nothing that could carry a
// path separator, a traversal, or a shell metacharacter does.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// ErrInvalidID is returned when an identifier is unusable as a path segment.
// Match with errors.Is.
var ErrInvalidID = errors.New("invalid identifier")

// ValidID reports whether id is safe to use as a path segment.
func ValidID(id string) bool {
	if id == "." || id == ".." {
		return false
	}
	return idPattern.MatchString(id)
}

// CheckID is ValidID with an error naming the offending value.
func CheckID(kind, id string) error {
	if !ValidID(id) {
		return fmt.Errorf("%s %q: %w", kind, id, ErrInvalidID)
	}
	return nil
}

// OrgsRoot is the parent of every organization tree.
func OrgsRoot(dataDir string) string {
	return filepath.Join(dataDir, OrgsDirName)
}

// OrgDir is one organization's tree.
func OrgDir(dataDir, orgID string) string {
	return filepath.Join(OrgsRoot(dataDir), orgID)
}

// OrgProteanDir is the org-global control directory.
func OrgProteanDir(dataDir, orgID string) string {
	return filepath.Join(OrgDir(dataDir, orgID), ControlDirName)
}

// OrgConfigsPath is the org-global configs.json.
func OrgConfigsPath(dataDir, orgID string) string {
	return filepath.Join(OrgProteanDir(dataDir, orgID), ConfigsFileName)
}

// OrgAgentsMD is the org-level AGENTS.md.
func OrgAgentsMD(dataDir, orgID string) string {
	return filepath.Join(OrgDir(dataDir, orgID), AgentsFileName)
}

// MemoriesDir is the root of the org's memories.
func MemoriesDir(dataDir, orgID string) string {
	return filepath.Join(OrgProteanDir(dataDir, orgID), MemoriesDirName)
}

// UserMemoryDir holds one user's memory, visible only to that user.
func UserMemoryDir(dataDir, orgID, userID string) string {
	return filepath.Join(MemoriesDir(dataDir, orgID), UserMemoriesDir, userID)
}

// UserMemoryPath is one user's MEMORY.md.
func UserMemoryPath(dataDir, orgID, userID string) string {
	return filepath.Join(UserMemoryDir(dataDir, orgID, userID), MemoryFileName)
}

// ProjectMemoryDir holds one project's memory, visible to everyone in the
// project. It lives at the org root, not inside the project tree, so the
// sandbox mask keeps it out of the agent's reach.
func ProjectMemoryDir(dataDir, orgID, projectID string) string {
	return filepath.Join(MemoriesDir(dataDir, orgID), ProjectMemoriesIn, projectID)
}

// ProjectMemoryPath is one project's MEMORY.md.
func ProjectMemoryPath(dataDir, orgID, projectID string) string {
	return filepath.Join(ProjectMemoryDir(dataDir, orgID, projectID), MemoryFileName)
}

// SkillsDir holds the org's installed skills.
func SkillsDir(dataDir, orgID string) string {
	return filepath.Join(OrgProteanDir(dataDir, orgID), SkillsDirName)
}

// SkillDir is one installed skill.
func SkillDir(dataDir, orgID, skillName string) string {
	return filepath.Join(SkillsDir(dataDir, orgID), skillName)
}

// SkillPath is one skill's SKILL.md, in the agentskills.io shape.
func SkillPath(dataDir, orgID, skillName string) string {
	return filepath.Join(SkillDir(dataDir, orgID, skillName), SkillFileName)
}

// ProjectsDir holds the org's projects.
func ProjectsDir(dataDir, orgID string) string {
	return filepath.Join(OrgDir(dataDir, orgID), ProjectsDirName)
}

// ProjectDir is one project's tree. It is what gets bind-mounted at the
// sandbox workspace root.
func ProjectDir(dataDir, orgID, projectID string) string {
	return filepath.Join(ProjectsDir(dataDir, orgID), projectID)
}

// ProjectProteanDir is the project's control directory, masked inside the
// sandbox.
func ProjectProteanDir(dataDir, orgID, projectID string) string {
	return filepath.Join(ProjectDir(dataDir, orgID, projectID), ControlDirName)
}

// ProjectConfigsPath is the project's configs.json, which merges over the org's.
func ProjectConfigsPath(dataDir, orgID, projectID string) string {
	return filepath.Join(ProjectProteanDir(dataDir, orgID, projectID), ConfigsFileName)
}

// ProjectAgentsMD is the project-level AGENTS.md.
func ProjectAgentsMD(dataDir, orgID, projectID string) string {
	return filepath.Join(ProjectDir(dataDir, orgID, projectID), AgentsFileName)
}

// ThreadsDir holds every conversation thread of one project.
func ThreadsDir(dataDir, orgID, projectID string) string {
	return filepath.Join(ProjectProteanDir(dataDir, orgID, projectID), ThreadsDirName)
}

// ThreadDir is one thread's directory.
func ThreadDir(dataDir, orgID, projectID, threadID string) string {
	return filepath.Join(ThreadsDir(dataDir, orgID, projectID), threadID)
}

// ThreadJSONPath is one thread's metadata and task list.
func ThreadJSONPath(dataDir, orgID, projectID, threadID string) string {
	return filepath.Join(ThreadDir(dataDir, orgID, projectID, threadID), ThreadFileName)
}

// MessagesDir holds one thread's message files.
func MessagesDir(dataDir, orgID, projectID, threadID string) string {
	return filepath.Join(ThreadDir(dataDir, orgID, projectID, threadID), MessagesDirName)
}

// MessagePath is one message file. Messages are append-only: the file is
// written once and never changed.
func MessagePath(dataDir, orgID, projectID, threadID, messageID string) string {
	return filepath.Join(MessagesDir(dataDir, orgID, projectID, threadID), messageID+MessageExtension)
}

// MessageIDFromFile turns a message file name back into its ID, reporting
// false for anything that is not a message file.
func MessageIDFromFile(name string) (string, bool) {
	id, ok := strings.CutSuffix(name, MessageExtension)
	if !ok || !ValidID(id) {
		return "", false
	}
	return id, true
}

// EnsureOrg creates the org skeleton: the control directory with its memories
// and skills roots, and an empty AGENTS.md if the org has none. It is
// idempotent and never truncates an existing file.
func EnsureOrg(dataDir, orgID string) error {
	if err := CheckID("org id", orgID); err != nil {
		return err
	}
	dirs := []struct {
		path string
		mode os.FileMode
	}{
		{OrgDir(dataDir, orgID), DirMode},
		{OrgProteanDir(dataDir, orgID), ControlDirMode},
		{filepath.Join(MemoriesDir(dataDir, orgID), UserMemoriesDir), ControlDirMode},
		{filepath.Join(MemoriesDir(dataDir, orgID), ProjectMemoriesIn), ControlDirMode},
		{SkillsDir(dataDir, orgID), ControlDirMode},
		{ProjectsDir(dataDir, orgID), DirMode},
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d.path, d.mode); err != nil {
			return fmt.Errorf("create %q: %w", d.path, err)
		}
	}
	return touch(OrgAgentsMD(dataDir, orgID))
}

// EnsureProject creates the org skeleton and then the project skeleton: the
// project directory, its control directory with the threads root, and an empty
// AGENTS.md if the project has none. It is idempotent.
func EnsureProject(dataDir, orgID, projectID string) error {
	if err := EnsureOrg(dataDir, orgID); err != nil {
		return err
	}
	if err := CheckID("project id", projectID); err != nil {
		return err
	}
	if err := os.MkdirAll(ProjectDir(dataDir, orgID, projectID), DirMode); err != nil {
		return fmt.Errorf("create %q: %w", ProjectDir(dataDir, orgID, projectID), err)
	}
	if err := os.MkdirAll(ThreadsDir(dataDir, orgID, projectID), ControlDirMode); err != nil {
		return fmt.Errorf("create %q: %w", ThreadsDir(dataDir, orgID, projectID), err)
	}
	return touch(ProjectAgentsMD(dataDir, orgID, projectID))
}

// touch creates path empty when it does not exist and leaves it alone
// otherwise.
func touch(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, FileMode)
	if err != nil {
		return fmt.Errorf("create %q: %w", path, err)
	}
	return f.Close()
}
