import { describe, expect, test } from "bun:test";
import type { FS } from "@protean/vfs";

import { discoverWorkspaceSkills, parseSkillFrontmatter } from "./skills";

describe("parseSkillFrontmatter", () => {
  test("parses standard frontmatter", () => {
    const content = `---
name: my-skill
description: A useful skill
---
# Rest of document`;

    expect(parseSkillFrontmatter(content)).toEqual({
      name: "my-skill",
      description: "A useful skill",
    });
  });

  test("parses arrays and metadata", () => {
    const content = `---
name: agent-skill
description: Handles agent work
allowed-tools:
  - Read
  - Bash
metadata:
  owner: team-core
  version: 2
---`;

    expect(parseSkillFrontmatter(content)).toEqual({
      "name": "agent-skill",
      "description": "Handles agent work",
      "allowed-tools": ["Read", "Bash"],
      "metadata": {
        owner: "team-core",
        version: "2",
      },
    });
  });

  test("returns null when no frontmatter exists", () => {
    expect(parseSkillFrontmatter("# markdown only")).toBeNull();
  });

  test("returns null for malformed yaml frontmatter", () => {
    const content = `---
name: "bad
---`;
    expect(parseSkillFrontmatter(content)).toBeNull();
  });
});

describe("discoverWorkspaceSkills", () => {
  function createMockFs(
    dirs: Record<string, { isDirectory: boolean }>,
    files: Record<string, string>,
  ): FS {
    return {
      readdir: async (dirPath: string) => {
        if (dirPath === "skills") {
          return Object.entries(dirs).map(([name, props]) => ({
            name,
            isDirectory: props.isDirectory,
          }));
        }
        throw new Error(`ENOENT: ${dirPath}`);
      },
      readFile: async (filePath: string) => {
        if (filePath in files) {
          return files[filePath];
        }
        throw new Error(`ENOENT: ${filePath}`);
      },
      stat: async () => {
        throw new Error("not implemented");
      },
      readFileBuffer: async () => {
        throw new Error("not implemented");
      },
      mkdir: async () => {},
      writeFile: async () => {},
      writeFileBuffer: async () => {},
      move: async () => {},
      remove: async () => {},
      resolvePath: (p: string) => p,
    };
  }

  test("discovers skills sorted by name", async () => {
    const fs = createMockFs(
      {
        "z-skill": { isDirectory: true },
        "a-skill": { isDirectory: true },
      },
      {
        "skills/z-skill/SKILL.md": `---
name: Zeta Skill
description: last
---`,
        "skills/a-skill/SKILL.md": `---
name: Alpha Skill
description: first
---`,
      },
    );

    const skills = await discoverWorkspaceSkills(fs);

    expect(skills).toHaveLength(2);
    expect(skills.map((s) => s.name)).toEqual(["Alpha Skill", "Zeta Skill"]);
  });

  test("falls back to directory name when frontmatter name is missing", async () => {
    const fs = createMockFs(
      { "my-dir": { isDirectory: true } },
      {
        "skills/my-dir/SKILL.md": `---
description: no name
---`,
      },
    );

    const skills = await discoverWorkspaceSkills(fs);

    expect(skills).toHaveLength(1);
    expect(skills[0]?.name).toBe("my-dir");
    expect(skills[0]?.description).toBe("no name");
    expect(skills[0]?.id).toBe("my-dir");
  });

  test("returns empty when skills directory is missing", async () => {
    const fs = createMockFs({}, {});
    fs.readdir = async () => {
      throw new Error("ENOENT");
    };

    expect(await discoverWorkspaceSkills(fs)).toEqual([]);
  });

  test("skips skills without frontmatter", async () => {
    const fs = createMockFs(
      { "no-front": { isDirectory: true } },
      {
        "skills/no-front/SKILL.md": "# No frontmatter",
      },
    );

    expect(await discoverWorkspaceSkills(fs)).toEqual([]);
  });
});
