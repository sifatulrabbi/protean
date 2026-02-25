import type { DynamicToolUIPart, ToolUIPart, UIMessage } from "ai";

export type ThreadStatus = "submitted" | "streaming" | "ready" | "error";

export type ThreadToolPart = ToolUIPart | DynamicToolUIPart;

export function isThreadToolPart(
  part: UIMessage["parts"][number],
): part is ThreadToolPart {
  return part.type === "dynamic-tool" || part.type.startsWith("tool-");
}

export function messageKeyFor(message: UIMessage, index: number): string {
  return message.id && message.id.trim().length > 0
    ? message.id
    : `${message.role}-${index}`;
}
