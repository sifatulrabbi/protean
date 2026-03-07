import { describe, expect, test } from "bun:test";
import { discoverSkills, parseFrontmatter } from "./skills";
import type { FS } from "@protean/vfs";

describe("parseFrontmatter", () => {
  test("parses valid frontmatter with name and description", () => {
    const content = `---
name: my-skill
description: A useful skill
---
# Rest of the document`;

    const result = parseFrontmatter(content);
    expect(result).toEqual({
      name: "my-skill",
      description: "A useful skill",
    });
  });

  test("parses nested metadata block", () => {
    const content = `---
name: my-skill
description: A useful skill
metadata:
  author: test-user
  version: 1.0
---
# Content`;

    const result = parseFrontmatter(content);
    expect(result).toEqual({
      name: "my-skill",
      description: "A useful skill",
      metadata: {
        author: "test-user",
        version: "1.0",
      },
    });
  });

  test("returns null when no frontmatter is present", () => {
    const content = "# Just a regular markdown file\nNo frontmatter here.";
    expect(parseFrontmatter(content)).toBeNull();
  });

  test("returns empty object for empty frontmatter", () => {
    const content = `---

---
Some content`;

    const result = parseFrontmatter(content);
    expect(result).toEqual({});
  });

  test("handles frontmatter with only name", () => {
    const content = `---
name: solo-skill
---`;

    const result = parseFrontmatter(content);
    expect(result).toEqual({ name: "solo-skill" });
  });
});

describe("discoverSkills", () => {
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
        if (filePath in files) return files[filePath];
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
description: The last skill
---
# Zeta`,
        "skills/a-skill/SKILL.md": `---
name: Alpha Skill
description: The first skill
---
# Alpha`,
      },
    );

    const skills = await discoverSkills(fs);
    expect(skills).toHaveLength(2);
    expect(skills[0].name).toBe("Alpha Skill");
    expect(skills[1].name).toBe("Zeta Skill");
    expect(skills[0].path).toBe("skills/a-skill/SKILL.md");
    expect(skills[1].description).toBe("The last skill");
  });

  test("returns empty array when skills directory does not exist", async () => {
    const fs = createMockFs({}, {});
    // Override readdir to throw for any path
    fs.readdir = async () => {
      throw new Error("ENOENT: no such file or directory");
    };

    const skills = await discoverSkills(fs);
    expect(skills).toEqual([]);
  });

  test("skips entries that fail to read", async () => {
    const fs = createMockFs(
      {
        "good-skill": { isDirectory: true },
        "bad-skill": { isDirectory: true },
      },
      {
        "skills/good-skill/SKILL.md": `---
name: Good Skill
description: Works fine
---`,
      },
      // bad-skill has no SKILL.md file, so readFile will throw
    );

    const skills = await discoverSkills(fs);
    expect(skills).toHaveLength(1);
    expect(skills[0].name).toBe("Good Skill");
  });

  test("skips non-directory entries", async () => {
    const fs = createMockFs(
      {
        "real-skill": { isDirectory: true },
        "README.md": { isDirectory: false },
      },
      {
        "skills/real-skill/SKILL.md": `---
name: Real Skill
description: A real skill
---`,
      },
    );

    const skills = await discoverSkills(fs);
    expect(skills).toHaveLength(1);
    expect(skills[0].name).toBe("Real Skill");
  });

  test("uses directory name when frontmatter has no name", () => {
    const fs = createMockFs(
      { "my-dir": { isDirectory: true } },
      {
        "skills/my-dir/SKILL.md": `---
description: No name field
---`,
      },
    );

    return discoverSkills(fs).then((skills) => {
      expect(skills).toHaveLength(1);
      expect(skills[0].name).toBe("my-dir");
      expect(skills[0].description).toBe("No name field");
    });
  });

  test("skips skills with no frontmatter", async () => {
    const fs = createMockFs(
      { "no-front": { isDirectory: true } },
      {
        "skills/no-front/SKILL.md": "# Just markdown\nNo frontmatter here.",
      },
    );

    const skills = await discoverSkills(fs);
    expect(skills).toEqual([]);
  });

  test("includes metadata when present", async () => {
    const fs = createMockFs(
      { "meta-skill": { isDirectory: true } },
      {
        "skills/meta-skill/SKILL.md": `---
name: Meta Skill
description: Has metadata
metadata:
  author: tester
  version: 2.0
---`,
      },
    );

    const skills = await discoverSkills(fs);
    expect(skills).toHaveLength(1);
    expect(skills[0].metadata).toEqual({
      author: "tester",
      version: "2.0",
    });
  });
});
