"use client";

import { useEffect, useState } from "react";
import { DownloadIcon } from "lucide-react";
import { Streamdown } from "streamdown";
import { cjk } from "@streamdown/cjk";
import { code } from "@streamdown/code";
import { math } from "@streamdown/math";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const IMAGE_EXTENSIONS = new Set(["png", "jpg", "jpeg", "gif", "webp", "svg"]);

const TEXT_EXTENSIONS = new Set([
  "txt",
  "md",
  "json",
  "csv",
  "html",
  "yaml",
  "yml",
  "xml",
  "ts",
  "tsx",
  "js",
  "jsx",
  "css",
  "py",
  "sh",
  "toml",
  "ini",
  "cfg",
  "env",
  "log",
]);

function getExtension(name: string): string {
  const dot = name.lastIndexOf(".");
  return dot >= 0 ? name.slice(dot + 1).toLowerCase() : "";
}

function isImage(ext: string) {
  return IMAGE_EXTENSIONS.has(ext);
}

function isText(ext: string) {
  return TEXT_EXTENSIONS.has(ext);
}

function isMarkdown(ext: string) {
  return ext === "md";
}

interface FileViewerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileName: string;
  filePath: string;
}

const streamdownPlugins = { cjk, code, math };

export function FileViewerDialog({
  open,
  onOpenChange,
  fileName,
  filePath,
}: FileViewerDialogProps) {
  const ext = getExtension(fileName);
  const serveUrl = `/api/files/${encodeURIComponent(filePath)}`;

  const [textContent, setTextContent] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      setTextContent(null);
      setError(null);
      return;
    }

    if (!isText(ext)) return;

    let cancelled = false;
    setLoading(true);
    setError(null);

    fetch(serveUrl)
      .then((res) => {
        if (!res.ok) throw new Error("Failed to load file");
        return res.text();
      })
      .then((text) => {
        if (!cancelled) {
          // Normalize literal \n and \t sequences that some tools write
          const normalized = text.replace(/\\n/g, "\n").replace(/\\t/g, "\t");
          setTextContent(normalized);
        }
      })
      .catch((err) => {
        if (!cancelled) setError(err.message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [open, serveUrl, ext]);

  const handleDownload = () => {
    const a = document.createElement("a");
    a.href = serveUrl;
    a.download = fileName;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  };

  const canView = isImage(ext) || isText(ext);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-4xl">
        <DialogHeader>
          <div className="flex items-center gap-2 pr-8">
            <DialogTitle className="min-w-0 truncate">{fileName}</DialogTitle>
            <Button
              variant="ghost"
              size="sm"
              className="shrink-0 gap-1.5"
              onClick={handleDownload}
            >
              <DownloadIcon className="size-3.5" />
              Download
            </Button>
          </div>
          <DialogDescription className="sr-only">
            Viewing {fileName}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-auto">
          {!canView && (
            <div className="flex flex-col items-center justify-center gap-3 py-12 text-center">
              <p className="text-sm text-muted-foreground">
                Preview not available for .{ext} files
              </p>
              <Button variant="outline" size="sm" onClick={handleDownload}>
                <DownloadIcon className="size-3.5" />
                Download file
              </Button>
            </div>
          )}

          {isImage(ext) && (
            <div className="flex items-center justify-center">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={serveUrl}
                alt={fileName}
                className="max-h-[70vh] max-w-full rounded object-contain"
              />
            </div>
          )}

          {isText(ext) && loading && (
            <p className="py-8 text-center text-sm text-muted-foreground">
              Loading...
            </p>
          )}

          {isText(ext) && error && (
            <p className="py-8 text-center text-sm text-destructive">{error}</p>
          )}

          {isText(ext) && !loading && !error && textContent !== null && (
            <>
              {isMarkdown(ext) ? (
                <Tabs defaultValue="preview">
                  <TabsList>
                    <TabsTrigger value="preview">Preview</TabsTrigger>
                    <TabsTrigger value="raw">Raw</TabsTrigger>
                  </TabsList>
                  <TabsContent value="preview" className="mt-2">
                    <div className="rounded-md border p-4">
                      <Streamdown
                        className="message-response size-full text-sm [&>*:first-child]:mt-0 [&>*:last-child]:mb-0"
                        plugins={streamdownPlugins}
                      >
                        {textContent}
                      </Streamdown>
                    </div>
                  </TabsContent>
                  <TabsContent value="raw" className="mt-2">
                    <pre className="max-h-[60vh] overflow-auto rounded-md border bg-muted/50 p-4 text-xs">
                      {textContent}
                    </pre>
                  </TabsContent>
                </Tabs>
              ) : (
                <pre className="max-h-[70vh] overflow-auto rounded-md border bg-muted/50 p-4 text-xs">
                  {textContent}
                </pre>
              )}
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
