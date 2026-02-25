"use client";

import { useEffect, useRef } from "react";
import {
  ArrowLeftIcon,
  FileIcon,
  FolderIcon,
  RefreshCwIcon,
} from "lucide-react";

import { useWorkspaceFilesActions } from "@/components/chat/actions/use-workspace-files-actions";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { FileViewerDialog } from "@/components/chat/file-viewer-dialog";
import {
  FileEntryContextMenu,
  type FileEntry,
} from "@/components/chat/file-entry-context-menu";
import { useWorkspaceFilesStore } from "@/components/chat/state/workspace-files-store";

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

interface RenameDialogProps {
  entry: FileEntry | null;
  onCancel: () => void;
  onConfirm: (newName: string) => void;
}

function RenameDialog({ entry, onCancel, onConfirm }: RenameDialogProps) {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (entry) {
      // Select filename without extension for files.
      setTimeout(() => {
        if (!inputRef.current) return;
        inputRef.current.focus();
        const dotIdx = entry.isDirectory ? -1 : entry.name.lastIndexOf(".");
        inputRef.current.setSelectionRange(
          0,
          dotIdx > 0 ? dotIdx : entry.name.length,
        );
      }, 0);
    }
  }, [entry]);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const formData = new FormData(e.currentTarget as HTMLFormElement);
    const rawValue = formData.get("newName");
    const trimmed = typeof rawValue === "string" ? rawValue.trim() : "";
    if (trimmed && trimmed !== entry?.name) {
      onConfirm(trimmed);
    } else {
      onCancel();
    }
  }

  return (
    <Dialog open={entry !== null} onOpenChange={(open) => !open && onCancel()}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Rename</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <Input
            defaultValue={entry?.name ?? ""}
            name="newName"
            ref={inputRef}
            className="mt-1"
          />
          <DialogFooter className="mt-4">
            <Button type="button" variant="ghost" onClick={onCancel}>
              Cancel
            </Button>
            <Button type="submit">Rename</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function WorkspaceFilesPanel() {
  const {
    confirmRename,
    deleteEntry,
    downloadEntry,
    openEntry,
    refreshEntries,
  } = useWorkspaceFilesActions();

  const currentDir = useWorkspaceFilesStore((state) => state.currentDir);
  const entries = useWorkspaceFilesStore((state) => state.entries);
  const error = useWorkspaceFilesStore((state) => state.error);
  const loading = useWorkspaceFilesStore((state) => state.loading);
  const viewerFile = useWorkspaceFilesStore((state) => state.viewerFile);
  const renameEntry = useWorkspaceFilesStore((state) => state.renameEntry);
  const navigateUp = useWorkspaceFilesStore((state) => state.navigateUp);
  const openRename = useWorkspaceFilesStore((state) => state.openRename);
  const closeRename = useWorkspaceFilesStore((state) => state.closeRename);
  const closeViewer = useWorkspaceFilesStore((state) => state.closeViewer);

  useEffect(() => {
    void refreshEntries(currentDir);
  }, [currentDir, refreshEntries]);

  function handleOpen(entry: FileEntry) {
    openEntry(entry);
  }

  function handleAddToChat(entry: FileEntry) {
    void entry;
    // TODO: implement add-to-chat.
  }

  function handleRenameRequest(entry: FileEntry) {
    openRename(entry);
  }

  return (
    <>
      <Sidebar side="right" collapsible="offcanvas">
        <SidebarHeader className="border-b px-3 py-2">
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={navigateUp}
              disabled={currentDir === "/"}
              className="size-7 p-0"
            >
              <ArrowLeftIcon className="size-4" />
              <span className="sr-only">Go back</span>
            </Button>

            <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
              /{currentDir === "/" ? "" : currentDir}
            </span>

            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                void refreshEntries(currentDir);
              }}
              className="size-7 p-0"
            >
              <RefreshCwIcon className="size-4" />
              <span className="sr-only">Refresh</span>
            </Button>
          </div>
        </SidebarHeader>

        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupContent>
              {loading && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  Loading...
                </p>
              )}

              {error ? (
                <p className="px-2 py-3 text-center text-destructive text-xs">
                  {error}
                </p>
              ) : null}

              {!loading && entries.length === 0 && (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  No files found
                </p>
              )}

              {!loading && entries.length > 0 && (
                <SidebarMenu>
                  {entries.map((entry) => (
                    <SidebarMenuItem key={entry.path}>
                      <FileEntryContextMenu
                        entry={entry}
                        onOpen={handleOpen}
                        onDownload={downloadEntry}
                        onAddToChat={handleAddToChat}
                        onDelete={(nextEntry) => {
                          void deleteEntry(nextEntry);
                        }}
                        onRename={handleRenameRequest}
                      >
                        <SidebarMenuButton onClick={() => handleOpen(entry)}>
                          {entry.isDirectory ? (
                            <FolderIcon className="size-4" />
                          ) : (
                            <FileIcon className="size-4" />
                          )}
                          <span className="truncate">{entry.name}</span>
                          {!entry.isDirectory ? (
                            <span className="ml-auto shrink-0 text-xs text-muted-foreground">
                              {formatBytes(entry.size)}
                            </span>
                          ) : null}
                        </SidebarMenuButton>
                      </FileEntryContextMenu>
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              )}
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
      </Sidebar>

      <FileViewerDialog
        open={viewerFile !== null}
        onOpenChange={(open) => !open && closeViewer()}
        fileName={viewerFile?.name ?? ""}
        filePath={viewerFile?.path ?? ""}
      />

      <RenameDialog
        entry={renameEntry}
        onConfirm={(newName) => {
          void confirmRename(newName);
        }}
        onCancel={closeRename}
      />
    </>
  );
}
