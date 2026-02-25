"use client";

import { create } from "zustand";
import type { FileEntry } from "@/components/chat/file-entry-context-menu";
import type { WorkspaceFilesState } from "@/components/chat/state/types";

interface WorkspaceFilesStore extends WorkspaceFilesState {
  closeRename: () => void;
  closeViewer: () => void;
  navigateUp: () => void;
  openRename: (entry: FileEntry) => void;
  openViewer: (entry: FileEntry) => void;
  reset: () => void;
  setCurrentDir: (dir: string) => void;
  setEntries: (entries: FileEntry[]) => void;
  setError: (error: string | null) => void;
  setLoading: (loading: boolean) => void;
}

const initialState: WorkspaceFilesState = {
  currentDir: "/",
  entries: [],
  error: null,
  loading: false,
  renameEntry: null,
  viewerFile: null,
};

export const useWorkspaceFilesStore = create<WorkspaceFilesStore>()((set) => ({
  ...initialState,

  closeRename: () => set({ renameEntry: null }),

  closeViewer: () => set({ viewerFile: null }),

  navigateUp: () =>
    set((state) => {
      if (state.currentDir === "/") {
        return state;
      }

      const parts = state.currentDir.split("/").filter(Boolean);
      parts.pop();
      return { currentDir: parts.length > 0 ? parts.join("/") : "/" };
    }),

  openRename: (renameEntry) => set({ renameEntry }),

  openViewer: (viewerFile) => set({ viewerFile }),

  reset: () => set(() => ({ ...initialState })),

  setCurrentDir: (currentDir) => set({ currentDir }),

  setEntries: (entries) => set({ entries }),

  setError: (error) => set({ error }),

  setLoading: (loading) => set({ loading }),
}));
