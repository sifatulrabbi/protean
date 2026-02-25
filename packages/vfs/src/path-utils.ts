import { resolve, relative, sep } from "path";

export function resolveWithinRoot(root: string, inputPath: string): string {
  const sanitized = inputPath.trim();
  const relativeInput = sanitized.startsWith("/")
    ? sanitized.slice(1)
    : sanitized;
  const candidate = resolve(root, relativeInput);
  const rel = relative(root, candidate);
  const escapesRoot = rel.startsWith("..") || rel.includes(`..${sep}`);

  if (escapesRoot) {
    throw new Error(`Path "${inputPath}" escapes workspace root.`);
  }

  return candidate;
}
