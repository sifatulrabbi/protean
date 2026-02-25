"use client";

import { useCallback } from "react";
import type { FileEntry } from "@/components/chat/file-entry-context-menu";
import {
  deleteEntry,
  downloadUrl,
  listEntries,
  renameEntry,
} from "@/components/chat/services/workspace-files-api-client";
import { useWorkspaceFilesStore } from "@/components/chat/state/workspace-files-store";

function triggerDownload(url: string, fileName: string): void {
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
}

export function useWorkspaceFilesActions() {
  const refreshEntries = useCallback(async (dir?: string): Promise<void> => {
    const store = useWorkspaceFilesStore.getState();
    const targetDir = dir ?? store.currentDir;

    store.setLoading(true);
    store.setError(null);

    try {
      const entries = await listEntries(targetDir);
      store.setEntries(entries);
    } catch {
      store.setEntries([]);
      store.setError("Failed to load directory");
    } finally {
      store.setLoading(false);
    }
  }, []);

  const openEntry = useCallback((entry: FileEntry): void => {
    if (entry.isDirectory) {
      useWorkspaceFilesStore.getState().setCurrentDir(entry.path);
      return;
    }

    useWorkspaceFilesStore.getState().openViewer(entry);
  }, []);

  const deleteEntryFromWorkspace = useCallback(
    async (entry: FileEntry): Promise<void> => {
      const ok = await deleteEntry(entry.path);
      if (!ok) {
        useWorkspaceFilesStore.getState().setError("Failed to delete entry");
        return;
      }

      await refreshEntries();
    },
    [refreshEntries],
  );

  const confirmRename = useCallback(
    async (newName: string): Promise<void> => {
      const store = useWorkspaceFilesStore.getState();
      const target = store.renameEntry;
      if (!target) {
        return;
      }

      const ok = await renameEntry(target.path, newName);
      store.closeRename();
      if (!ok) {
        store.setError("Failed to rename entry");
        return;
      }

      await refreshEntries();
    },
    [refreshEntries],
  );

  const downloadEntry = useCallback((entry: FileEntry): void => {
    triggerDownload(downloadUrl(entry.path), entry.name);
  }, []);

  return {
    confirmRename,
    deleteEntry: deleteEntryFromWorkspace,
    downloadEntry,
    openEntry,
    refreshEntries,
  };
}
