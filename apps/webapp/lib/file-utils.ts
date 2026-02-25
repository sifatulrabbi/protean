export interface DetectedFile {
  path: string;
  name: string;
  extension: string;
  previewable: boolean;
  isDirectory: boolean;
  serveUrl: string;
  sizeBytes?: number;
}

const PREVIEWABLE_EXTENSIONS = new Set([
  "txt",
  "md",
  "json",
  "csv",
  "html",
  "yaml",
  "yml",
  "png",
  "jpg",
  "jpeg",
  "gif",
  "webp",
  "svg",
]);

const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "webp", "svg"]);

export function isPreviewable(ext: string): boolean {
  return PREVIEWABLE_EXTENSIONS.has(ext.toLowerCase());
}

export function isImageExtension(ext: string): boolean {
  return IMAGE_EXTENSIONS.has(ext.toLowerCase());
}

function extractExtension(filePath: string): string {
  const dot = filePath.lastIndexOf(".");
  return dot >= 0 ? filePath.slice(dot + 1).toLowerCase() : "";
}

function extractName(filePath: string): string {
  const slash = filePath.lastIndexOf("/");
  return slash >= 0 ? filePath.slice(slash + 1) : filePath;
}

function toWorkspaceRelative(fullPath: string): string {
  // Strip everything up to and including /project/{userId}/
  const marker = "/project/";
  const idx = fullPath.indexOf(marker);
  if (idx < 0) return fullPath;
  const afterMarker = fullPath.slice(idx + marker.length);
  // Skip the userId segment
  const slashIdx = afterMarker.indexOf("/");
  return slashIdx >= 0 ? afterMarker.slice(slashIdx + 1) : afterMarker;
}

function buildFile(
  rawPath: string,
  opts?: { sizeBytes?: number; isDirectory?: boolean },
): DetectedFile {
  const relativePath = toWorkspaceRelative(rawPath);
  const ext = extractExtension(relativePath);
  return {
    path: relativePath,
    name: extractName(relativePath),
    extension: ext,
    previewable: isPreviewable(ext),
    isDirectory: opts?.isDirectory ?? false,
    serveUrl: `/api/files/${encodeURIComponent(relativePath)}`,
    sizeBytes: opts?.sizeBytes,
  };
}

/**
 * Detect files created by tool outputs.
 * Recognizes shapes from WriteFile, Move, GenerateDocxFromCode,
 * DocxToMarkdown, DocxToImages, and ModifyDocxWithJson.
 */
export function detectFilesFromToolResult(
  output: unknown,
): DetectedFile[] | null {
  if (!output || typeof output !== "object") return null;

  const obj = output as Record<string, unknown>;
  const files: DetectedFile[] = [];

  // Mkdir → { fullPath } without bytesWritten
  // WriteFile → { fullPath, bytesWritten }
  if (typeof obj.fullPath === "string" && obj.fullPath.length > 0) {
    const isDir =
      typeof obj.bytesWritten === "undefined" &&
      !extractExtension(obj.fullPath);
    const size =
      typeof obj.bytesWritten === "number" ? obj.bytesWritten : undefined;
    files.push(
      buildFile(obj.fullPath, { sizeBytes: size, isDirectory: isDir }),
    );
  }

  // Move/rename → { sourceFullPath, destinationFullPath, moved }
  // Surface destination path so UI reflects the new file name/location.
  if (
    obj.moved === true &&
    typeof obj.destinationFullPath === "string" &&
    obj.destinationFullPath.length > 0
  ) {
    files.push(buildFile(obj.destinationFullPath));
  }

  // GenerateDocxFromCode / ModifyDocxWithJson → { outputPath }
  if (
    typeof obj.outputPath === "string" &&
    obj.outputPath.length > 0 &&
    !obj.fullPath
  ) {
    files.push(buildFile(obj.outputPath));
  }

  // DocxToMarkdown → { markdownPath }
  if (typeof obj.markdownPath === "string" && obj.markdownPath.length > 0) {
    files.push(buildFile(obj.markdownPath));
  }

  // DocxToImages → { pages: string[] }
  if (Array.isArray(obj.pages)) {
    for (const page of obj.pages) {
      if (typeof page === "string" && page.length > 0) {
        files.push(buildFile(page));
      }
    }
  }

  return files.length > 0 ? files : null;
}
