import { countTokenFast } from "@protean/utils";

import type {
  ContextSize,
  ThreadMessageRecord,
  ThreadPricingCalculator,
  ThreadUsage,
} from "./types";

/** Returns a zeroed-out {@link ThreadUsage} object. Used as the initial accumulator. */
export function emptyUsage(): ThreadUsage {
  return {
    inputTokens: 0,
    outputTokens: 0,
    totalDurationMs: 0,
    totalCostUsd: 0,
  };
}

/** Returns a zeroed-out {@link ContextSize} value. */
export function emptyContextSize(): ContextSize {
  return 0;
}

/**
 * Sums token counts, duration, and cost across all non-deleted messages in `history`.
 *
 * Soft-deleted messages (`deletedAt !== null`) are excluded so that their
 * tokens don't inflate the reported totals after compaction.
 *
 * @param history - The full `ThreadRecord.history` array.
 */
export function aggregateThreadUsage(
  history: ThreadMessageRecord[],
): ThreadUsage {
  const activeMessages = history.filter(
    (message) => message.deletedAt === null,
  );
  const totalInputTokens = countTokenFast(
    ...activeMessages.map((message) => messagePartsToText(message)),
  );
  const usageTotals = activeMessages.reduce<ThreadUsage>(
    (acc, message) => ({
      ...acc,
      outputTokens: acc.outputTokens + message.usage.outputTokens,
      totalDurationMs: acc.totalDurationMs + message.usage.totalDurationMs,
      totalCostUsd: acc.totalCostUsd + message.usage.totalCostUsd,
    }),
    emptyUsage(),
  );

  return {
    ...usageTotals,
    inputTokens: totalInputTokens,
  };
}

/**
 * Extracts all the text content of tool context as string from the thread message.
 */
function messagePartsToText(message: ThreadMessageRecord): string {
  const textContents: string[] = [];

  message.message.parts.forEach((part) => {
    if (part.type === "text" && typeof part.text === "string") {
      textContents.push(part.text);
    }

    if (part.type === "reasoning" && typeof part.text === "string") {
      textContents.push(part.text);
    }

    if (part.type === "dynamic-tool") {
      if (typeof part.input === "string") {
        textContents.push(part.input);
      } else if (typeof part.input === "object") {
        textContents.push(JSON.stringify(part.input));
      }

      if (typeof part.output === "string") {
        textContents.push(part.output);
      } else if (typeof part.output === "object") {
        textContents.push(JSON.stringify(part.output));
      }
    }
  });

  return textContents.join("\n");
}

/**
 * Computes the active context-window token counts from non-deleted messages.
 *
 * Unlike {@link aggregateThreadUsage}, this is typically called on the
 * result of `deriveActiveHistory(thread)` rather than the full `history`
 * to reflect what the model currently sees in its context window.
 *
 * @param history - Usually the output of `deriveActiveHistory(thread)`.
 */
export function aggregateContextSize(history: ThreadMessageRecord[]) {
  const activeMessages = history.filter(
    (message) => message.deletedAt === null,
  );
  return countTokenFast(
    ...activeMessages.map((msg) => messagePartsToText(msg)),
  );
}

/**
 * Determines the USD cost for a single message.
 *
 * Resolution order:
 * 1. Use `explicitCostUsd` if the caller already knows the cost (e.g. from an API response).
 * 2. Delegate to `pricingCalculator` if one was provided at `createFsMemory` time.
 * 3. Return `0` as a safe default when neither is available.
 */
export function resolveMessageCost(args: {
  inputTokens: number;
  outputTokens: number;
  explicitCostUsd?: number;
  modelId?: string;
  pricingCalculator?: ThreadPricingCalculator;
}): number {
  if (typeof args.explicitCostUsd === "number") {
    return args.explicitCostUsd;
  }

  if (!args.pricingCalculator) {
    return 0;
  }

  return args.pricingCalculator.calculateCost({
    modelId: args.modelId ?? "unknown",
    inputTokens: args.inputTokens,
    outputTokens: args.outputTokens,
  });
}
