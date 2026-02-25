export const description =
  "Core filesystem skill. Read, write, and navigate files and directories in the sandboxed workspace. Any task that involves reading files, listing directories, creating files, or writing content. This is the foundational skill - most other skills depend on it.";

export const instructions = `# Workspace Skill

You have access to a sandboxed project workspace through the tools below.
All paths are resolved relative to the workspace root unless an absolute path is given.

## Available Tools

### Stat

Get metadata (size, type, timestamps) for files/directories.
Use this to check whether a path is a file or directory, or to inspect size before reading.

### ListDir

List directory entries.
Use this to explore what exists at a location and distinguish files from directories.

### ReadFile

Read file contents.
Only use on text files (source code, config, markdown, etc.).
For large files, check Stat first to avoid reading unexpectedly large content.

### Mkdir

Create directories.
Creates a directory at the given path, including any missing intermediate directories.

### WriteFile

Write content to files.
Creates the file if it doesn't exist, overwrites if it does.
Always confirm with the user before overwriting existing files.

### Move

Move/rename files or directories.
Use this when reorganizing files or renaming existing paths.

### Remove

Delete files or directories.

## Guidelines

- Read before you write: inspect existing files before modifying them.
- Use Stat to check size before reading large files.
- Prefer ListDir when you need to tell files from directories.
- Use Move for renaming or relocating existing files/directories instead of re-writing content.
- Never write to paths outside the workspace root.`;
