"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import type { UIMessage } from "ai";
import type { ModelSelection } from "@protean/model-catalog";
import {
  getModelReasoningById,
  resolveClientModelSelection,
} from "@protean/model-catalog";
import { useModelCatalog } from "@/components/chat/model-catalog-provider";
import {
  addMessage,
  createThread,
  editMessage,
  updateThreadModelSelection,
} from "@/components/chat/services/thread-api-client";
import { useComposerStore } from "@/components/chat/state/composer-store";
import { useThreadSessionStore } from "@/components/chat/state/thread-session-store";
import type { ThreadActionResult } from "@/components/chat/state/types";

interface EditUserMessageArgs {
  messageId: string;
  modelSelection?: ModelSelection;
  text: string;
}

interface RerunAssistantMessageArgs {
  messageId: string;
  modelSelection?: ModelSelection;
}

async function persistThreadModelSelection(
  selection: ModelSelection,
): Promise<void> {
  const threadId = useThreadSessionStore.getState().activeThreadId;
  if (!threadId) {
    return;
  }

  await updateThreadModelSelection({
    modelSelection: selection,
    threadId,
  });
}

export function useThreadActions() {
  const router = useRouter();
  const providers = useModelCatalog();

  const refreshSidebar = useCallback(() => {
    router.refresh();
  }, [router]);

  const applyInvocationModelSelection = useCallback(
    async (
      overrideSelection?: ModelSelection,
    ): Promise<ModelSelection | undefined> => {
      if (!overrideSelection) {
        return undefined;
      }

      const resolved = resolveClientModelSelection(
        providers,
        useComposerStore.getState().modelSelection,
        overrideSelection,
      );

      useComposerStore.getState().setModelSelection(resolved);
      await persistThreadModelSelection(resolved);

      return resolved;
    },
    [providers],
  );

  const submitPrompt = useCallback(
    async ({ text }: { text: string }): Promise<ThreadActionResult> => {
      const trimmedText = text.trim();
      if (!trimmedText) {
        return { ok: true };
      }

      const sessionStore = useThreadSessionStore.getState();
      const composerStore = useComposerStore.getState();
      const runtime = sessionStore.runtime;

      if (!runtime) {
        const error = new Error("Chat runtime is not initialized");
        sessionStore.setError(error);
        return { error, ok: false };
      }

      const userMessage: UIMessage = {
        id: crypto.randomUUID(),
        role: "user",
        parts: [{ type: "text", text: trimmedText }],
      };

      if (!sessionStore.activeThreadId) {
        sessionStore.setIsCreatingThread(true);
        runtime.setMessages((currentMessages) => [
          ...currentMessages,
          userMessage,
        ]);

        try {
          const threadId = await createThread({
            initialUserMessage: trimmedText,
            modelSelection: composerStore.modelSelection,
            title: trimmedText.slice(0, 60),
          });

          sessionStore.setActiveThreadId(threadId);
          router.replace(`/chats/t/${threadId}`);
          refreshSidebar();
          return { ok: true };
        } catch (error) {
          runtime.setMessages((currentMessages) =>
            currentMessages.filter((message) => message.id !== userMessage.id),
          );

          const normalizedError =
            error instanceof Error
              ? error
              : new Error("Unable to create and submit prompt");
          sessionStore.setError(normalizedError);
          return { error: normalizedError, ok: false };
        } finally {
          useThreadSessionStore.getState().setIsCreatingThread(false);
        }
      }

      runtime.setMessages((currentMessages) => [
        ...currentMessages,
        userMessage,
      ]);
      sessionStore.setIsPersistingMutation(true);
      let messagePersisted = false;
      try {
        await addMessage({
          threadId: sessionStore.activeThreadId,
          message: userMessage,
          modelSelection: composerStore.modelSelection,
        });
        messagePersisted = true;

        await runtime.sendMessage({
          messageId: userMessage.id,
          text: trimmedText,
        });
        refreshSidebar();
        return { ok: true };
      } catch (error) {
        if (!messagePersisted) {
          runtime.setMessages((currentMessages) =>
            currentMessages.filter((message) => message.id !== userMessage.id),
          );
        }

        const normalizedError =
          error instanceof Error ? error : new Error("Unable to submit prompt");
        sessionStore.setError(normalizedError);
        return { error: normalizedError, ok: false };
      } finally {
        useThreadSessionStore.getState().setIsPersistingMutation(false);
      }
    },
    [refreshSidebar, router],
  );

  const editUserMessage = useCallback(
    async ({
      messageId,
      modelSelection,
      text,
    }: EditUserMessageArgs): Promise<ThreadActionResult> => {
      const trimmedText = text.trim();
      if (!trimmedText) {
        return { ok: true };
      }

      const sessionStore = useThreadSessionStore.getState();
      const runtime = sessionStore.runtime;
      const threadId = sessionStore.activeThreadId;

      if (!runtime || !threadId) {
        const error = new Error("Thread is not ready for edits");
        sessionStore.setError(error);
        return { error, ok: false };
      }

      sessionStore.setIsPersistingMutation(true);
      try {
        const invocationModelSelection =
          await applyInvocationModelSelection(modelSelection);

        const editedMessage: UIMessage = {
          id: messageId,
          role: "user",
          parts: [{ type: "text", text: trimmedText }],
        };

        await editMessage({
          threadId,
          messageId,
          message: editedMessage,
        });

        await runtime.sendMessage(
          {
            messageId,
            text: trimmedText,
          },
          invocationModelSelection
            ? {
                body: {
                  modelSelection: invocationModelSelection,
                },
              }
            : undefined,
        );
        refreshSidebar();
        return { ok: true };
      } catch (error) {
        const normalizedError =
          error instanceof Error ? error : new Error("Unable to edit message");
        sessionStore.setError(normalizedError);
        return { error: normalizedError, ok: false };
      } finally {
        useThreadSessionStore.getState().setIsPersistingMutation(false);
      }
    },
    [applyInvocationModelSelection, refreshSidebar],
  );

  const rerunAssistantMessage = useCallback(
    async ({
      messageId,
      modelSelection,
    }: RerunAssistantMessageArgs): Promise<ThreadActionResult> => {
      const sessionStore = useThreadSessionStore.getState();
      const runtime = sessionStore.runtime;

      if (!runtime) {
        const error = new Error("Thread is not ready for reruns");
        sessionStore.setError(error);
        return { error, ok: false };
      }

      try {
        const invocationModelSelection =
          await applyInvocationModelSelection(modelSelection);

        await runtime.regenerate({
          ...(invocationModelSelection
            ? {
                body: {
                  modelSelection: invocationModelSelection,
                },
              }
            : {}),
          messageId,
        });
        refreshSidebar();
        return { ok: true };
      } catch (error) {
        const normalizedError =
          error instanceof Error
            ? error
            : new Error("Unable to rerun response");
        sessionStore.setError(normalizedError);
        return { error: normalizedError, ok: false };
      }
    },
    [applyInvocationModelSelection, refreshSidebar],
  );

  const changeModel = useCallback(
    (selection: ModelSelection): void => {
      const reasoning = getModelReasoningById(
        providers,
        selection.providerId,
        selection.modelId,
      );

      const resolved: ModelSelection = {
        ...selection,
        reasoningBudget:
          (reasoning?.defaultValue as ModelSelection["reasoningBudget"]) ??
          "none",
      };

      useComposerStore.getState().setModelSelection(resolved);
      void persistThreadModelSelection(resolved);
    },
    [providers],
  );

  const changeReasoningBudget = useCallback((budget: string): void => {
    useComposerStore.getState().setReasoningBudget(budget);
    void persistThreadModelSelection(
      useComposerStore.getState().modelSelection,
    );
  }, []);

  const stop = useCallback((): void => {
    useThreadSessionStore.getState().runtime?.stop();
  }, []);

  return {
    changeModel,
    changeReasoningBudget,
    editUserMessage,
    rerunAssistantMessage,
    stop,
    submitPrompt,
  };
}
