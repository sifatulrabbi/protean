"use client";

import type { UIMessage } from "ai";
import type { ModelSelection } from "@protean/model-catalog";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { useShallow } from "zustand/react/shallow";
import { getModelById } from "@protean/model-catalog";
import { useModelCatalog } from "@/components/chat/model-catalog-provider";
import { useThreadActions } from "@/components/chat/actions/use-thread-actions";
import { ThreadEditPanel } from "@/components/chat/thread-edit-panel";
import { ThreadMessageList } from "@/components/chat/thread-message-list";
import { ThreadRerunDialog } from "@/components/chat/thread-rerun-dialog";
import {
  selectIsBusy,
  useThreadSessionStore,
} from "@/components/chat/state/thread-session-store";
import { useComposerStore } from "@/components/chat/state/composer-store";
import { useMessageUiStore } from "@/components/chat/state/message-ui-store";
import { useRenderCountDebug } from "@/components/chat/utils/use-render-count-debug";
import { getMessageText } from "@/components/chat/utils/chat-message-utils";
import { ThreadToast } from "@/components/chat/thread-toast";

export function ThreadMessages() {
  useRenderCountDebug("ThreadMessages");

  const { editUserMessage, rerunAssistantMessage } = useThreadActions();
  const providers = useModelCatalog();

  const { messageUsageMap, messages, status } = useThreadSessionStore(
    useShallow((state) => ({
      messageUsageMap: state.messageUsageMap,
      messages: state.messages,
      status: state.status,
    })),
  );
  const isBusy = useThreadSessionStore(selectIsBusy);
  const currentModelSelection = useComposerStore(
    (state) => state.modelSelection,
  );

  const {
    copiedMessageId,
    editingMessageId,
    editModelSelection,
    editReasoningBudget,
    editValue,
    rerunMessageId,
    rerunModelSelection,
    toast,
  } = useMessageUiStore(
    useShallow((state) => ({
      copiedMessageId: state.copiedMessageId,
      editingMessageId: state.editingMessageId,
      editModelSelection: state.editModelSelection,
      editReasoningBudget: state.editReasoningBudget,
      editValue: state.editValue,
      rerunMessageId: state.rerunMessageId,
      rerunModelSelection: state.rerunModelSelection,
      toast: state.toast,
    })),
  );

  const setCopiedMessage = useMessageUiStore((state) => state.setCopiedMessage);
  const startEditStore = useMessageUiStore((state) => state.startEdit);
  const cancelEdit = useMessageUiStore((state) => state.cancelEdit);
  const setEditValue = useMessageUiStore((state) => state.setEditValue);
  const setEditModelSelection = useMessageUiStore(
    (state) => state.setEditModelSelection,
  );
  const setEditReasoningBudget = useMessageUiStore(
    (state) => state.setEditReasoningBudget,
  );
  const openRerun = useMessageUiStore((state) => state.openRerun);
  const closeRerun = useMessageUiStore((state) => state.closeRerun);
  const setRerunModelSelection = useMessageUiStore(
    (state) => state.setRerunModelSelection,
  );
  const showToastStore = useMessageUiStore((state) => state.showToast);
  const clearToast = useMessageUiStore((state) => state.clearToast);

  const toastTimeoutRef = useRef<number | null>(null);
  const copiedTimeoutRef = useRef<number | null>(null);

  const effectiveEditModelSelection =
    editModelSelection ?? currentModelSelection;
  const selectedEditModel = useMemo(
    () =>
      getModelById(
        providers,
        effectiveEditModelSelection.providerId,
        effectiveEditModelSelection.modelId,
      ),
    [
      effectiveEditModelSelection.modelId,
      effectiveEditModelSelection.providerId,
      providers,
    ],
  );
  const availableEditReasoningBudgets = useMemo(
    () => selectedEditModel?.reasoning.budgets ?? [],
    [selectedEditModel],
  );
  const supportsEditThinking = availableEditReasoningBudgets.some(
    (budget) => budget !== "none",
  );
  const activeEditReasoningBudget = useMemo(() => {
    const defaultValue = selectedEditModel?.reasoning.defaultValue ?? "none";
    if (!editReasoningBudget) return defaultValue;
    if (!selectedEditModel?.reasoning.budgets.includes(editReasoningBudget)) {
      return defaultValue;
    }
    return editReasoningBudget;
  }, [editReasoningBudget, selectedEditModel]);

  const showToast = useCallback(
    (nextToast: {
      description?: string;
      title: string;
      variant: "default" | "destructive";
    }) => {
      showToastStore(nextToast);

      if (toastTimeoutRef.current !== null) {
        window.clearTimeout(toastTimeoutRef.current);
      }

      toastTimeoutRef.current = window.setTimeout(() => {
        clearToast();
      }, 3000);
    },
    [clearToast, showToastStore],
  );

  useEffect(
    () => () => {
      if (toastTimeoutRef.current !== null) {
        window.clearTimeout(toastTimeoutRef.current);
      }
      if (copiedTimeoutRef.current !== null) {
        window.clearTimeout(copiedTimeoutRef.current);
      }
    },
    [],
  );

  const copyMessage = useCallback(
    async (message: UIMessage) => {
      const text = getMessageText(message);
      if (!text) {
        return;
      }

      try {
        await navigator.clipboard.writeText(text);
      } catch {
        return;
      }

      setCopiedMessage(message.id);

      if (copiedTimeoutRef.current !== null) {
        window.clearTimeout(copiedTimeoutRef.current);
      }

      copiedTimeoutRef.current = window.setTimeout(() => {
        const currentCopied = useMessageUiStore.getState().copiedMessageId;
        if (currentCopied === message.id) {
          useMessageUiStore.getState().setCopiedMessage(null);
        }
      }, 1500);
    },
    [setCopiedMessage],
  );

  const startEdit = useCallback(
    (message: UIMessage) => {
      startEditStore({
        currentModelSelection,
        message,
      });
    },
    [currentModelSelection, startEditStore],
  );

  const saveEdit = useCallback(async () => {
    const store = useMessageUiStore.getState();
    if (!store.editingMessageId) {
      return;
    }

    const base = store.editModelSelection ?? currentModelSelection;
    const payload = {
      messageId: store.editingMessageId,
      modelSelection: {
        ...base,
        reasoningBudget: (store.editReasoningBudget ??
          selectedEditModel?.reasoning.defaultValue ??
          base.reasoningBudget) as ModelSelection["reasoningBudget"],
      },
      text: store.editValue,
    };

    store.cancelEdit();

    const result = await editUserMessage(payload);
    if (result.ok) {
      showToast({
        title: "Message updated",
        variant: "default",
      });
      return;
    }

    showToast({
      description: "Please try again.",
      title: "Could not save message",
      variant: "destructive",
    });
  }, [currentModelSelection, editUserMessage, selectedEditModel, showToast]);

  const openRerunDialog = useCallback(
    (message: UIMessage) => {
      openRerun({
        currentModelSelection,
        messageId: message.id,
      });
    },
    [currentModelSelection, openRerun],
  );

  const confirmRerun = useCallback(async () => {
    const store = useMessageUiStore.getState();
    if (!store.rerunMessageId) {
      return;
    }

    const payload: {
      messageId: string;
      modelSelection?: ModelSelection;
    } = {
      messageId: store.rerunMessageId,
      modelSelection: store.rerunModelSelection ?? currentModelSelection,
    };

    store.closeRerun();

    const result = await rerunAssistantMessage(payload);
    if (result.ok) {
      showToast({
        title: "Rerun started",
        variant: "default",
      });
      return;
    }

    showToast({
      description: "Please try again.",
      title: "Could not rerun response",
      variant: "destructive",
    });
  }, [currentModelSelection, rerunAssistantMessage, showToast]);

  const editPanel = (
    <ThreadEditPanel
      activeEditReasoningBudget={activeEditReasoningBudget}
      availableEditReasoningBudgets={availableEditReasoningBudgets}
      editValue={editValue}
      hasMessageId={Boolean(editingMessageId && editingMessageId.trim())}
      isBusy={isBusy}
      onCancel={cancelEdit}
      onModelSelectionChange={setEditModelSelection}
      onReasoningBudgetChange={setEditReasoningBudget}
      onSave={() => {
        void saveEdit();
      }}
      onValueChange={setEditValue}
      supportsEditThinking={supportsEditThinking}
      value={editModelSelection ?? currentModelSelection}
    />
  );

  return (
    <>
      <ThreadMessageList
        copiedMessageId={copiedMessageId}
        editPanel={editPanel}
        editingMessageId={editingMessageId}
        isBusy={isBusy}
        messageUsageMap={messageUsageMap}
        messages={messages}
        onCopyMessage={(message) => {
          void copyMessage(message);
        }}
        onOpenRerunDialog={openRerunDialog}
        onStartEdit={startEdit}
        status={status}
      />

      <ThreadRerunDialog
        currentModelSelection={currentModelSelection}
        isBusy={isBusy}
        onClose={closeRerun}
        onConfirm={() => {
          void confirmRerun();
        }}
        onRerunModelSelectionChange={setRerunModelSelection}
        open={Boolean(rerunMessageId)}
        rerunMessageId={rerunMessageId}
        rerunModelSelection={rerunModelSelection}
      />

      <ThreadToast toast={toast} />
    </>
  );
}
