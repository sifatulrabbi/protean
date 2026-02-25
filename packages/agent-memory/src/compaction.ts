import { randomUUID } from "node:crypto";
import type { UIMessage } from "ai";

import { aggregateContextSize } from "./usage";
import { deriveActiveHistory } from "./derive-active-history";
import type {
  CompactThreadOptions,
  CompactThreadResult,
  ThreadMessageRecord,
  ThreadRecord,
} from "./types";

/**
 * Internal service that decides whether a thread needs compaction and
 * performs it when required.
 *
 * Compaction works by:
 * 1. Calling the caller-supplied `summarizeHistory` function to produce a
 *    single summary `UIMessage` from the current active history.
 * 2. Appending a synthetic summary record to `history` and advancing
 *    `lastCompactionOrdinal` so that `deriveActiveHistory` returns only
 *    the summary going forward.
 * 3. Old messages remain in `history` for auditing and debugging.
 */
export interface HistoryCompactor {
  compactIfNeeded(
    thread: ThreadRecord,
    options: CompactThreadOptions,
  ): Promise<CompactThreadResult>;
  shouldCompact(
    thread: ThreadRecord,
    policy: { maxContextTokens: number; reservedOutputTokens?: number },
  ): boolean;
}

/**
 * Factory that creates a {@link HistoryCompactor} instance.
 * Keeping this as a factory (rather than a class) makes it easy to inject
 * dependencies or swap the implementation in tests.
 */
export function createHistoryCompactor(): HistoryCompactor {
  /**
   * Returns `true` when the combined token count of the active context window
   * plus the reserved output budget exceeds `maxContextTokens`.
   */
  function shouldCompact(
    thread: ThreadRecord,
    policy: CompactThreadOptions["policy"],
  ): boolean {
    const active = deriveActiveHistory(thread);
    const used = aggregateContextSize(active);
    const reserved = policy.reservedOutputTokens ?? 0;

    return used + reserved > policy.maxContextTokens;
  }

  /**
   * Runs compaction if the policy threshold is crossed; otherwise returns the
   * thread unchanged with `didCompact: false`.
   *
   * When compaction occurs:
   * - The active history is passed to `options.summarizeHistory` to generate
   *   a summary message.
   * - A synthetic summary record is appended to `history`.
   * - `lastCompactionOrdinal` is set to the ordinal of the most recent
   *   message *before* the summary, so `deriveActiveHistory` returns only
   *   the new summary record going forward.
   */
  async function compactIfNeeded(
    thread: ThreadRecord,
    options: CompactThreadOptions,
  ): Promise<CompactThreadResult> {
    const needsCompaction = shouldCompact(thread, options.policy);

    if (!needsCompaction) {
      return {
        didCompact: false,
        thread,
      };
    }

    const now = options.now ?? new Date().toISOString();
    const activeHistory = deriveActiveHistory(thread);
    const summaryMessage = await options.summarizeHistory(activeHistory);

    const latestOrdinal =
      thread.history.length > 0
        ? (thread.history[thread.history.length - 1]?.ordinal ?? 0)
        : 0;

    const summaryOrdinal = latestOrdinal + 1;
    const summaryRecord = buildSummaryRecord(
      summaryMessage,
      now,
      summaryOrdinal,
    );

    const nextHistory = [...thread.history, summaryRecord];

    const compactedThread: ThreadRecord = {
      ...thread,
      history: nextHistory,
      lastCompactionOrdinal: latestOrdinal,
      contextSize: aggregateContextSize([summaryRecord]),
      updatedAt: now,
    };

    return {
      didCompact: true,
      thread: compactedThread,
    };
  }

  return {
    compactIfNeeded,
    shouldCompact,
  };
}

/** Tool name used for the synthetic compaction tool-call in history. */
const COMPACTION_TOOL_NAME = "AutoCompactHistory";

/**
 * Wraps a summary `UIMessage` in a `ThreadMessageRecord` suitable for
 * appending to `history`.
 *
 * The record is stored as an `assistant` message with a single
 * `dynamic-tool` part in `output-available` state. This lets the frontend
 * render the compaction as a visible tool-call step without any special
 * casing — the existing chain-of-thought UI picks it up automatically.
 *
 * Usage fields are zeroed out — the summary message itself doesn't
 * represent a real model call.
 */
function buildSummaryRecord(
  message: UIMessage,
  now: string,
  ordinal: number,
): ThreadMessageRecord {
  const summaryText = extractSummaryText(message);
  const toolCallId = randomUUID();

  const normalized: UIMessage = {
    id: randomUUID(),
    role: "assistant",
    parts: [
      {
        type: "dynamic-tool",
        toolName: COMPACTION_TOOL_NAME,
        toolCallId,
        state: "output-available",
        input: { reason: "Context window limit exceeded" },
        output: { summary: summaryText },
      },
    ],
  };

  return {
    id: randomUUID(),
    ordinal,
    version: 1,
    message: normalized,
    usage: {
      inputTokens: 0,
      outputTokens: 0,
      totalDurationMs: 0,
      totalCostUsd: 0,
    },
    createdAt: now,
    updatedAt: now,
    deletedAt: null,
    error: null,
  };
}

/**
 * Extracts a plain string from a `UIMessage` for use as the compaction
 * tool output.
 *
 * Text parts are concatenated with newlines. Non-text parts are preserved as
 * JSON snippets to avoid silently dropping potentially relevant context.
 */
function extractSummaryText(message: UIMessage): string {
  const parts: string[] = [];
  for (const part of message.parts) {
    if (part.type === "text" && typeof part.text === "string") {
      parts.push(part.text);
      continue;
    }

    parts.push(JSON.stringify(part));
  }

  return parts.join("\n").trim() || "Conversation summary.";
}
