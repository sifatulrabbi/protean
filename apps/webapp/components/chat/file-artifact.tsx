"use client";

import { useCallback } from "react";
import {
  DownloadIcon,
  FileIcon,
  FileTextIcon,
  FolderIcon,
  ImageIcon,
  FileSpreadsheetIcon,
  PresentationIcon,
} from "lucide-react";

import { type DetectedFile, isImageExtension } from "@/lib/file-utils";

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function FileTypeIcon({
  extension,
  isDirectory,
}: {
  extension: string;
  isDirectory: boolean;
}) {
  if (isDirectory)
    return (
      <FolderIcon className="size-3.5 shrink-0 text-muted-foreground/60" />
    );
  const ext = extension.toLowerCase();
  if (isImageExtension(ext))
    return <ImageIcon className="size-3.5 shrink-0 text-muted-foreground/60" />;
  if (ext === "csv" || ext === "xlsx")
    return (
      <FileSpreadsheetIcon className="size-3.5 shrink-0 text-muted-foreground/60" />
    );
  if (ext === "pptx")
    return (
      <PresentationIcon className="size-3.5 shrink-0 text-muted-foreground/60" />
    );
  if (["txt", "md", "json", "yaml", "yml", "html", "xml"].includes(ext))
    return (
      <FileTextIcon className="size-3.5 shrink-0 text-muted-foreground/60" />
    );
  return <FileIcon className="size-3.5 shrink-0 text-muted-foreground/60" />;
}

function downloadFile(url: string, fileName: string) {
  const a = document.createElement("a");
  a.href = url;
  a.download = fileName;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
}

interface FileArtifactProps {
  file: DetectedFile;
}

export function FileArtifact({ file }: FileArtifactProps) {
  const handleDownload = useCallback(() => {
    downloadFile(file.serveUrl, file.name);
  }, [file.serveUrl, file.name]);

  return (
    <div className="my-1.5 inline-flex items-center gap-2 rounded-md border border-border/60 px-3 py-1.5">
      <FileTypeIcon extension={file.extension} isDirectory={file.isDirectory} />
      <span className="text-sm text-muted-foreground">{file.name}</span>
      {file.sizeBytes != null && (
        <span className="text-xs text-muted-foreground/60">
          {formatBytes(file.sizeBytes)}
        </span>
      )}
      {!file.isDirectory && (
        <button
          type="button"
          onClick={handleDownload}
          className="ml-1 text-muted-foreground/50 hover:text-muted-foreground"
        >
          <DownloadIcon className="size-3.5" />
          <span className="sr-only">Download</span>
        </button>
      )}
    </div>
  );
}
