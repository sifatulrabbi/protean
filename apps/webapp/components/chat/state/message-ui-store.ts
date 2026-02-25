"use client";

import { create } from "zustand";
import type { ModelSelection } from "@protean/model-catalog";
import type { UIMessage } from "ai";
import type {
  MessageToastState,
  MessageUiState,
} from "@/components/chat/state/types";

function getMessageText(message: UIMessage): string {
  return message.parts
    .filter((part) => part.type === "text")
    .map((part) => part.text)
    .join("\n")
    .trim();
}

interface MessageUiStore extends MessageUiState {
  cancelEdit: () => void;
  clearToast: () => void;
  closeRerun: () => void;
  openRerun: (args: {
    currentModelSelection: ModelSelection;
    messageId: string;
  }) => void;
  setCopiedMessage: (messageId: string | null) => void;
  setEditModelSelection: (selection: ModelSelection | null) => void;
  setEditReasoningBudget: (budget: string | null) => void;
  setEditValue: (value: string) => void;
  setRerunModelSelection: (selection: ModelSelection | null) => void;
  showToast: (toast: MessageToastState) => void;
  reset: () => void;
  startEdit: (args: {
    currentModelSelection: ModelSelection;
    message: UIMessage;
  }) => void;
}

const initialState: MessageUiState = {
  copiedMessageId: null,
  editingMessageId: null,
  editModelSelection: null,
  editReasoningBudget: null,
  editValue: "",
  rerunMessageId: null,
  rerunModelSelection: null,
  toast: null,
};

export const useMessageUiStore = create<MessageUiStore>()((set) => ({
  ...initialState,

  cancelEdit: () =>
    set({
      editingMessageId: null,
      editModelSelection: null,
      editReasoningBudget: null,
      editValue: "",
    }),

  clearToast: () => set({ toast: null }),

  closeRerun: () => set({ rerunMessageId: null, rerunModelSelection: null }),

  openRerun: ({ currentModelSelection, messageId }) =>
    set({
      rerunMessageId: messageId,
      rerunModelSelection: currentModelSelection,
    }),

  setCopiedMessage: (copiedMessageId) => set({ copiedMessageId }),

  setEditModelSelection: (editModelSelection) => set({ editModelSelection }),

  setEditReasoningBudget: (editReasoningBudget) => set({ editReasoningBudget }),

  setEditValue: (editValue) => set({ editValue }),

  setRerunModelSelection: (rerunModelSelection) => set({ rerunModelSelection }),

  showToast: (toast) => set({ toast }),

  reset: () => set({ ...initialState }),

  startEdit: ({ currentModelSelection, message }) => {
    if (message.role !== "user") {
      return;
    }

    set({
      editingMessageId: message.id,
      editModelSelection: currentModelSelection,
      editReasoningBudget: currentModelSelection.reasoningBudget,
      editValue: getMessageText(message),
    });
  },
}));
