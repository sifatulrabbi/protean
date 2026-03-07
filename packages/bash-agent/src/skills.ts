import {
  discoverWorkspaceSkills,
  parseSkillFrontmatter,
  type SkillManifest,
} from "@protean/skill-manifests";
import type { FS } from "@protean/vfs";

export type { SkillManifest };

export const parseFrontmatter = parseSkillFrontmatter;

export async function discoverSkills(fs: FS): Promise<SkillManifest[]> {
  return discoverWorkspaceSkills(fs);
}
