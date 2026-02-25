"use client";

import { create } from "zustand";
import type { UIMessage } from "ai";
import type { ThreadUsage } from "@protean/agent-memory";
import type {
  ThreadRuntimeBridge,
  ThreadSessionState,
} from "@/components/chat/state/types";

interface ThreadSessionStore extends ThreadSessionState {
  appendOptimisticMessage: (message: UIMessage) => void;
  hydrateFromRoute: (args: {
    activeThreadId: string | null;
    messageUsageMap?: Record<string, ThreadUsage>;
    messages?: UIMessage[];
  }) => void;
  removeMessageById: (messageId: string) => void;
  resetForRoute: () => void;
  setActiveThreadId: (threadId: string | null) => void;
  setError: (error?: Error) => void;
  setIsCreatingThread: (isCreating: boolean) => void;
  setIsPersistingMutation: (isPersisting: boolean) => void;
  setMessages: (messages: UIMessage[]) => void;
  setPendingInvokeHandled: (threadId: string, handled: boolean) => void;
  setRuntime: (runtime: ThreadRuntimeBridge | null) => void;
  setStatus: (status: ThreadSessionState["status"]) => void;
  setUsageMap: (map: Record<string, ThreadUsage>) => void;
}

const initialState: ThreadSessionState = {
  activeThreadId: null,
  error: undefined,
  isCreatingThread: false,
  isPersistingMutation: false,
  messageUsageMap: {},
  messages: [],
  pendingInvokeHandled: {},
  runtime: null,
  status: "ready",
};

export const useThreadSessionStore = create<ThreadSessionStore>()((set) => ({
  ...initialState,

  appendOptimisticMessage: (message) =>
    set((state) => ({ messages: [...state.messages, message] })),

  hydrateFromRoute: ({ activeThreadId, messageUsageMap = {}, messages = [] }) =>
    set((state) => ({
      ...state,
      activeThreadId,
      error: undefined,
      isCreatingThread: false,
      isPersistingMutation: false,
      messageUsageMap,
      messages,
      status: "ready",
    })),

  removeMessageById: (messageId) =>
    set((state) => ({
      messages: state.messages.filter((message) => message.id !== messageId),
    })),

  resetForRoute: () => set(() => ({ ...initialState })),

  setActiveThreadId: (activeThreadId) => set({ activeThreadId }),

  setError: (error) => set({ error }),

  setIsCreatingThread: (isCreatingThread) => set({ isCreatingThread }),

  setIsPersistingMutation: (isPersistingMutation) =>
    set({ isPersistingMutation }),

  setMessages: (messages) => set({ messages }),

  setPendingInvokeHandled: (threadId, handled) =>
    set((state) => {
      if (handled) {
        return {
          pendingInvokeHandled: {
            ...state.pendingInvokeHandled,
            [threadId]: true,
          },
        };
      }

      const nextHandled = { ...state.pendingInvokeHandled };
      delete nextHandled[threadId];
      return { pendingInvokeHandled: nextHandled };
    }),

  setRuntime: (runtime) => set({ runtime }),

  setStatus: (status) => set({ status }),

  setUsageMap: (messageUsageMap) => set({ messageUsageMap }),
}));

export const selectIsBusy = (state: ThreadSessionState): boolean =>
  state.status === "submitted" ||
  state.status === "streaming" ||
  state.isCreatingThread ||
  state.isPersistingMutation;

export const selectCanSubmit = (state: ThreadSessionState): boolean =>
  !selectIsBusy(state);
