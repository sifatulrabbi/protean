"use client";

import { create } from "zustand";
import type { ThreadRecordTrimmed } from "@protean/agent-memory";
import type { ThreadsSidebarState } from "@/components/chat/state/types";

interface ThreadsSidebarStore extends ThreadsSidebarState {
  hydrateThreads: (threads: ThreadRecordTrimmed[]) => void;
  removeThread: (threadId: string) => void;
  setDeletingThreadId: (threadId: string | null) => void;
  setMobileOpen: (open: boolean) => void;
  setMounted: (mounted: boolean) => void;
}

const initialState: ThreadsSidebarState = {
  deletingThreadId: null,
  mobileOpen: false,
  mounted: false,
  threadItems: [],
};

export const useThreadsSidebarStore = create<ThreadsSidebarStore>()((set) => ({
  ...initialState,

  hydrateThreads: (threadItems) => set({ threadItems }),

  removeThread: (threadId) =>
    set((state) => ({
      threadItems: state.threadItems.filter((thread) => thread.id !== threadId),
    })),

  setDeletingThreadId: (deletingThreadId) => set({ deletingThreadId }),

  setMobileOpen: (mobileOpen) => set({ mobileOpen }),

  setMounted: (mounted) => set({ mounted }),
}));
