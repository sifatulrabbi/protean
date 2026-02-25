"use client";

import type { ReactNode } from "react";
import type { UIMessage } from "ai";
import {
  CheckIcon,
  CopyIcon,
  MessageSquareIcon,
  PencilIcon,
  RotateCcwIcon,
  SparklesIcon,
} from "lucide-react";
import type { ThreadUsage } from "@protean/agent-memory";
import {
  Conversation,
  ConversationContent,
  ConversationEmptyState,
  ConversationScrollButton,
} from "@/components/ai-elements/conversation";
import {
  Message,
  MessageAction,
  MessageActions,
  MessageContent,
  MessageToolbar,
} from "@/components/ai-elements/message";
import { ThreadMessageParts } from "@/components/chat/thread-message-parts";
import {
  messageKeyFor,
  type ThreadStatus,
} from "@/components/chat/thread-ui-shared";
import { getMessageText } from "@/components/chat/utils/chat-message-utils";
import { cn, formatCostUsd, formatTokenCount } from "@/lib/utils";
import { Spinner } from "@/components/ui/spinner";

function ThreadMessagesLoadingState() {
  return (
    <div className="flex flex-col items-center gap-4 rounded-xl border border-dashed border-border/70 bg-muted/20 px-6 py-10 text-center">
      <Spinner className="size-5 text-muted-foreground" />
      <div className="space-y-1">
        <p className="font-medium text-sm">Preparing your conversation</p>
        <p className="text-muted-foreground text-xs">
          Creating thread and starting your request...
        </p>
      </div>
    </div>
  );
}

interface ThreadMessageListProps {
  copiedMessageId: string | null;
  editPanel: ReactNode;
  editingMessageId: string | null;
  isBusy: boolean;
  messageUsageMap: Record<string, ThreadUsage>;
  messages: UIMessage[];
  onCopyMessage: (message: UIMessage) => void;
  onOpenRerunDialog: (message: UIMessage) => void;
  onStartEdit: (message: UIMessage) => void;
  status: ThreadStatus;
}

export function ThreadMessageList({
  copiedMessageId,
  editPanel,
  editingMessageId,
  isBusy,
  messageUsageMap,
  messages,
  onCopyMessage,
  onOpenRerunDialog,
  onStartEdit,
  status,
}: ThreadMessageListProps) {
  return (
    <Conversation className="min-h-0 flex-1">
      <ConversationContent>
        {messages.length === 0 ? (
          status === "submitted" ? (
            <ThreadMessagesLoadingState />
          ) : (
            <ConversationEmptyState
              description="Send a message to start the conversation."
              icon={<MessageSquareIcon className="size-6" />}
              title="Start a conversation"
            />
          )
        ) : (
          messages.map((message, messageIndex) => {
            const messageKey = messageKeyFor(message, messageIndex);
            const messageText = getMessageText(message);
            const hasMessageId = message.id.trim().length > 0;

            return (
              <Message from={message.role} key={messageKey}>
                <MessageContent>
                  <ThreadMessageParts
                    isLastMessage={messageIndex === messages.length - 1}
                    isStreaming={status === "streaming"}
                    message={message}
                    messageKey={messageKey}
                  />
                </MessageContent>

                {editingMessageId === message.id ? editPanel : null}

                {editingMessageId !== message.id &&
                !isBusy &&
                (message.role === "assistant" || message.role === "user") ? (
                  <MessageToolbar
                    className={
                      message.role === "user" ? "mt-0 justify-end" : "mt-0"
                    }
                  >
                    <MessageActions>
                      <MessageAction
                        disabled={!messageText}
                        label="Copy message"
                        onClick={() => onCopyMessage(message)}
                        tooltip="Copy"
                      >
                        {copiedMessageId === message.id ? (
                          <CheckIcon className="size-4" />
                        ) : (
                          <CopyIcon className="size-4" />
                        )}
                      </MessageAction>

                      {message.role === "user" ? (
                        <MessageAction
                          disabled={isBusy || !hasMessageId}
                          label="Edit message"
                          onClick={() => onStartEdit(message)}
                          tooltip="Edit"
                        >
                          <PencilIcon className="size-4" />
                        </MessageAction>
                      ) : null}

                      {message.role === "assistant" ? (
                        <MessageAction
                          disabled={isBusy || !hasMessageId}
                          label="Rerun response"
                          onClick={() => onOpenRerunDialog(message)}
                          tooltip="Rerun"
                        >
                          <RotateCcwIcon className="size-4" />
                        </MessageAction>
                      ) : null}
                    </MessageActions>

                    {(() => {
                      const usage = messageUsageMap[message.id];
                      if (!usage || message.role !== "assistant") {
                        return null;
                      }
                      const cost = formatCostUsd(usage.totalCostUsd);
                      return (
                        <div className="flex items-center gap-1.5 text-muted-foreground/70 text-xs">
                          <span>{formatTokenCount(usage.inputTokens)} in</span>
                          <span>·</span>
                          <span>
                            {formatTokenCount(usage.outputTokens)} out
                          </span>
                          {cost ? (
                            <>
                              <span>·</span>
                              <span>{cost}</span>
                            </>
                          ) : null}
                        </div>
                      );
                    })()}
                  </MessageToolbar>
                ) : null}
              </Message>
            );
          })
        )}
        {messages.length > 0 ? (
          <div className="flex justify-start">
            <SparklesIcon
              className={cn(
                "size-4 transition-colors duration-700",
                isBusy ? "animate-spark-cycle" : "text-muted-foreground/50",
              )}
            />
          </div>
        ) : null}
      </ConversationContent>
      <ConversationScrollButton />
    </Conversation>
  );
}
