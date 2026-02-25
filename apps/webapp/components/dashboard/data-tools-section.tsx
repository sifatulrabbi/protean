"use client";

import { type ChangeEvent, useRef, useState } from "react";
import { DownloadIcon, UploadIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function DataToolsSection() {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [isExporting, setIsExporting] = useState(false);
  const [importMessage, setImportMessage] = useState<string | null>(null);

  const handleExport = async () => {
    setIsExporting(true);

    try {
      const response = await fetch("/threads", { method: "GET" });

      if (!response.ok) {
        setImportMessage("Unable to export data right now.");
        return;
      }

      const exportData = await response.text();
      const blob = new Blob([exportData], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = "chat-data-export.json";
      document.body.append(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      setImportMessage("Export complete.");
    } finally {
      setIsExporting(false);
    }
  };

  const handleImportClick = () => {
    fileInputRef.current?.click();
  };

  const handleImportFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];

    if (!file) {
      return;
    }

    try {
      const content = await file.text();
      const parsed = JSON.parse(content) as { threads?: unknown[] };

      if (!Array.isArray(parsed.threads)) {
        setImportMessage("Invalid file format. Expected exported chat JSON.");
      } else {
        setImportMessage(
          `Imported ${parsed.threads.length} threads (preview only).`,
        );
      }
    } catch {
      setImportMessage("Invalid JSON file.");
    } finally {
      event.target.value = "";
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Data</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-muted-foreground text-sm">
          Export your chat data or preview-import an existing export file.
        </p>
        <div className="grid gap-2 sm:grid-cols-2">
          <Button
            disabled={isExporting}
            onClick={handleExport}
            type="button"
            variant="outline"
          >
            <DownloadIcon className="size-4" />
            {isExporting ? "Exporting..." : "Export data"}
          </Button>
          <Button onClick={handleImportClick} type="button" variant="outline">
            <UploadIcon className="size-4" />
            Import data
          </Button>
        </div>
        <input
          accept="application/json"
          className="hidden"
          onChange={handleImportFile}
          ref={fileInputRef}
          type="file"
        />
        {importMessage ? (
          <p className="text-muted-foreground text-xs">{importMessage}</p>
        ) : null}
      </CardContent>
    </Card>
  );
}
