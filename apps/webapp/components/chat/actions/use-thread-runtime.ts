"use client";

import { useEffect, useMemo } from "react";
import { useChat } from "@ai-sdk/react";
import { DefaultChatTransport, type UIMessage } from "ai";
import type {
  ModelSelection,
  AIModelProviderEntry,
} from "@protean/model-catalog";
import { resolveClientModelSelection } from "@protean/model-catalog";
import type { ThreadUsage } from "@protean/agent-memory";
import { useComposerStore } from "@/components/chat/state/composer-store";
import { useMessageUiStore } from "@/components/chat/state/message-ui-store";
import { useThreadSessionStore } from "@/components/chat/state/thread-session-store";
import { isPendingMessage } from "@/components/chat/utils/chat-message-utils";

interface UseThreadRuntimeArgs {
  defaultModelSelection: ModelSelection;
  initialMessageUsageMap?: Record<string, ThreadUsage>;
  initialMessages: UIMessage[];
  initialModelSelection?: ModelSelection;
  initialThreadId?: string;
  providers: AIModelProviderEntry[];
}

export function useThreadRuntime({
  defaultModelSelection,
  initialMessageUsageMap,
  initialMessages,
  initialModelSelection,
  initialThreadId,
  providers,
}: UseThreadRuntimeArgs): void {
  const initialSelection = useMemo(
    () =>
      resolveClientModelSelection(
        providers,
        defaultModelSelection,
        initialModelSelection,
      ),
    [defaultModelSelection, initialModelSelection, providers],
  );

  useEffect(() => {
    useComposerStore
      .getState()
      .hydrateComposer({ modelSelection: initialSelection });

    useThreadSessionStore.getState().hydrateFromRoute({
      activeThreadId: initialThreadId ?? null,
      messageUsageMap: initialMessageUsageMap,
      messages: initialMessages,
    });

    useMessageUiStore.getState().reset();
  }, [
    initialMessageUsageMap,
    initialMessages,
    initialSelection,
    initialThreadId,
  ]);

  const transport = useMemo(
    () =>
      new DefaultChatTransport<UIMessage>({
        api: "/agent/chat",
        prepareSendMessagesRequest: ({ api, body, id, trigger }) => {
          const bodyAsRecord = body as Record<string, unknown> | undefined;
          const modelSelectionFromBody = bodyAsRecord?.modelSelection as
            | ModelSelection
            | undefined;

          return {
            api,
            body: {
              ...body,
              id,
              modelSelection:
                modelSelectionFromBody ??
                useComposerStore.getState().modelSelection,
              threadId: useThreadSessionStore.getState().activeThreadId,
              trigger,
            },
          };
        },
      }),
    [],
  );

  const {
    error,
    messages,
    regenerate,
    sendMessage,
    setMessages,
    status,
    stop,
  } = useChat({
    messages: initialMessages,
    transport,
  });

  useEffect(() => {
    useThreadSessionStore.getState().setRuntime({
      regenerate,
      sendMessage,
      setMessages,
      stop,
    });

    return () => {
      useThreadSessionStore.getState().setRuntime(null);
    };
  }, [regenerate, sendMessage, setMessages, stop]);

  useEffect(() => {
    useThreadSessionStore.getState().setMessages(messages);
  }, [messages]);

  useEffect(() => {
    useThreadSessionStore.getState().setStatus(status);
  }, [status]);

  useEffect(() => {
    useThreadSessionStore.getState().setError(error as Error | undefined);
  }, [error]);

  useEffect(() => {
    if (!initialThreadId) return;

    const store = useThreadSessionStore.getState();
    if (store.pendingInvokeHandled[initialThreadId]) {
      return;
    }

    const hasPendingMessage = initialMessages.some(
      (message) => message.role === "user" && isPendingMessage(message),
    );

    if (!hasPendingMessage) {
      return;
    }

    store.setPendingInvokeHandled(initialThreadId, true);

    void (async () => {
      try {
        await sendMessage(undefined, {
          body: { invokePending: true },
        });
      } catch {
        useThreadSessionStore
          .getState()
          .setPendingInvokeHandled(initialThreadId, false);
      }
    })();
  }, [initialMessages, initialThreadId, sendMessage]);
}
