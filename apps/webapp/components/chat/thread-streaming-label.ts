import type { UIMessage } from "ai";
import {
  isThreadToolPart,
  type ThreadStatus,
} from "@/components/chat/thread-ui-shared";

export function getStreamingLabel(
  status: ThreadStatus,
  messages: UIMessage[],
): string | null {
  if (status === "submitted") {
    return "Generating answer";
  }

  if (status !== "streaming") {
    return null;
  }

  const lastMessage = messages.at(-1);
  if (!lastMessage || lastMessage.role !== "assistant") {
    return "Generating answer";
  }

  const lastPart = lastMessage.parts.at(-1);
  if (lastPart?.type === "reasoning") {
    return "Reasoning";
  }

  if (
    lastPart &&
    isThreadToolPart(lastPart) &&
    ["input-streaming", "input-available", "approval-requested"].includes(
      lastPart.state,
    )
  ) {
    return "Using tools";
  }

  return "Generating answer";
}
