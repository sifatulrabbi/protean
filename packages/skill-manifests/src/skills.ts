import { parseDocument } from "yaml";
import type { FS } from "@protean/vfs";

export interface SkillFrontmatter {
  name?: string;
  description?: string;
  metadata?: Record<string, string>;
  [key: string]: unknown;
}

export interface SkillManifest {
  id: string;
  name: string;
  description: string;
  path: string;
  frontmatter: SkillFrontmatter;
  metadata?: Record<string, string>;
}

const FRONTMATTER_PATTERN = /^\ufeff?---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/;

function toStringRecord(value: unknown): Record<string, string> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }

  const record = Object.entries(value).reduce<Record<string, string>>(
    (acc, [key, entryValue]) => {
      if (typeof entryValue === "string") {
        acc[key] = entryValue;
      } else if (
        typeof entryValue === "number" ||
        typeof entryValue === "boolean"
      ) {
        acc[key] = String(entryValue);
      }
      return acc;
    },
    {},
  );

  return Object.keys(record).length > 0 ? record : undefined;
}

export function parseSkillFrontmatter(
  content: string,
): SkillFrontmatter | null {
  const match = content.match(FRONTMATTER_PATTERN);
  if (!match) {
    return null;
  }

  try {
    const document = parseDocument(match[1], { schema: "failsafe" });
    if (document.errors.length > 0) {
      return null;
    }

    const parsed = document.toJSON();
    if (parsed === null || parsed === undefined) {
      return {};
    }

    if (typeof parsed !== "object" || Array.isArray(parsed)) {
      return {};
    }

    const frontmatter = parsed as SkillFrontmatter;
    const metadata = toStringRecord(frontmatter.metadata);

    return {
      ...frontmatter,
      ...(metadata ? { metadata } : {}),
    };
  } catch {
    return null;
  }
}

export async function discoverWorkspaceSkills(
  fs: FS,
  options?: {
    skillsDir?: string;
    skillFileName?: string;
  },
): Promise<SkillManifest[]> {
  const skillsDir = options?.skillsDir ?? "skills";
  const skillFileName = options?.skillFileName ?? "SKILL.md";

  let entries: Array<{ name: string; isDirectory: boolean }>;
  try {
    entries = await fs.readdir(skillsDir);
  } catch {
    return [];
  }

  const manifests: SkillManifest[] = [];

  for (const entry of entries) {
    if (!entry.isDirectory) {
      continue;
    }

    const skillPath = `${skillsDir}/${entry.name}/${skillFileName}`;

    try {
      const content = await fs.readFile(skillPath);
      const frontmatter = parseSkillFrontmatter(content);
      if (!frontmatter) {
        continue;
      }

      const parsedName =
        typeof frontmatter.name === "string" ? frontmatter.name.trim() : "";
      const parsedDescription =
        typeof frontmatter.description === "string"
          ? frontmatter.description.trim()
          : "";

      manifests.push({
        id: entry.name,
        name: parsedName || entry.name,
        description: parsedDescription,
        path: skillPath,
        frontmatter,
        ...(frontmatter.metadata ? { metadata: frontmatter.metadata } : {}),
      });
    } catch {
      continue;
    }
  }

  return manifests.sort((a, b) => a.name.localeCompare(b.name));
}
