import { type UIMessageChunk } from "ai";

export function formatChunk(chunk: UIMessageChunk): string {
  switch (chunk.type) {
    case "start":
      return `\n[ui:start] messageId=${chunk.messageId ?? "n/a"}\n`;
    case "start-step":
      return `\n[ui:step:start]\n`;
    case "reasoning-start":
      return `\n[reasoning:start] id=${chunk.id}\n`;
    case "reasoning-delta":
      return `${chunk.delta}`;
    case "reasoning-end":
      return `\n[reasoning:end] id=${chunk.id}\n`;
    case "tool-input-start":
      return `\n[tool:input:start] tool=${chunk.toolName} callId=${chunk.toolCallId}\n`;
    case "tool-input-delta":
      return `${chunk.inputTextDelta}`;
    case "tool-input-available":
      //  input=${JSON.stringify(chunk.input)}
      return `\n[tool:call] tool=${chunk.toolName} callId=${chunk.toolCallId}\n`;
    case "tool-output-available":
      return `\n[tool:response] callId=${chunk.toolCallId} output=${JSON.stringify(chunk.output)}\n`;
    case "tool-output-error":
      return `\n[tool:error] callId=${chunk.toolCallId} error=${chunk.errorText}\n`;
    case "tool-output-denied":
      return `\n[tool:denied] callId=${chunk.toolCallId}\n`;
    case "text-start":
      return `\n[text:start] id=${chunk.id}\n`;
    case "text-delta":
      return `${chunk.delta}`;
    case "text-end":
      return `\n[text:end] id=${chunk.id}\n`;
    case "finish-step":
      return `\n[ui:step:finish]\n`;
    case "finish":
      return `\n[ui:finish] reason=${chunk.finishReason ?? "unknown"}\n`;
    case "abort":
      return `\n[ui:abort] reason=${chunk.reason ?? "unknown"}\n`;
    case "error":
      return `\n[ui:error] ${chunk.errorText}\n`;
    default:
      return `\n[ui:${chunk.type}] ${JSON.stringify(chunk)}\n`;
  }
}
