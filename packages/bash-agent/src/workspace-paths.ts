import { relative, resolve, sep } from "node:path";

export function normalizeWorkspaceRoot(workspaceRoot: string): string {
  return resolve(workspaceRoot);
}

export function resolveWithinWorkspace(
  workspaceRoot: string,
  targetPath = ".",
): string {
  const root = normalizeWorkspaceRoot(workspaceRoot);
  const resolvedPath = resolve(root, targetPath);
  const rel = relative(root, resolvedPath);
  const escapesRoot = rel === ".." || rel.startsWith(`..${sep}`);

  if (escapesRoot) {
    throw new Error(`Path "${targetPath}" escapes workspace root.`);
  }

  return resolvedPath;
}

export function toWorkspaceRelativePath(
  workspaceRoot: string,
  absolutePath: string,
): string {
  const root = normalizeWorkspaceRoot(workspaceRoot);
  const rel = relative(root, absolutePath);

  if (!rel || rel === ".") {
    return ".";
  }

  return rel.split(sep).join("/");
}
