import type { UIMessage } from "ai";

/** Accumulated token counts and cost for a single message or an entire thread. */
export interface ThreadUsage {
  inputTokens: number;
  outputTokens: number;
  totalDurationMs: number;
  totalCostUsd: number;
}

/**
 * Snapshot of the active context window size.
 * Tracks total tokens currently visible to the model
 * (derived from `history` + `lastCompactionOrdinal`).
 */
export type ContextSize = number;

/**
 * A single persisted message within a thread.
 *
 * - `ordinal` is a monotonically increasing integer assigned at insert time
 *   and used to reconstruct ordering after compaction.
 * - `version` is incremented when the record itself is mutated in place.
 * - `deletedAt` enables soft-deletion: the record stays on disk but is
 *   excluded from the derived active history and context-size calculations.
 */
export interface ThreadMessageRecord {
  id: string;
  ordinal: number;
  version: number;
  message: UIMessage;
  usage: ThreadUsage;
  createdAt: string;
  updatedAt: string;
  deletedAt: string | null;
  error: string | null;
}

/**
 * The root object written to each `.threads/thread.<id>.json` file.
 *
 * - `history`        — append-only log of every message ever saved, including
 *                      compaction summaries.
 * - `lastCompactionOrdinal` — ordinal up to which messages were compacted.
 *                             The active context window is derived at runtime
 *                             via `deriveActiveHistory(thread)`.
 * - `schemaVersion` / `contentSchemaVersion` — bumped when the file format or
 *   the message content format changes, enabling forward-compatible migrations.
 */
export interface ThreadRecord {
  schemaVersion: number;
  contentSchemaVersion: number;
  id: string;
  userId: string;
  title: string;
  modelSelection: ModelSelection;
  history: ThreadMessageRecord[];
  lastCompactionOrdinal: number | null;
  contextSize: ContextSize;
  usage: ThreadUsage;
  createdAt: string;
  updatedAt: string;
  deletedAt: string | null;
}

export type ThreadRecordTrimmed = Omit<ThreadRecord, "history">;

/**
 * @deprecated Import from `@protean/model-catalog` instead.
 */
export { reasoningBudgets } from "@protean/model-catalog";

/**
 * @deprecated Import from `@protean/model-catalog` instead.
 */
export type { ReasoningBudget, ModelSelection } from "@protean/model-catalog";

import type { ModelSelection } from "@protean/model-catalog";

/**
 * @deprecated Use {@link ModelSelection} from `@protean/model-catalog` instead.
 */
export type ThreadModelSelection = ModelSelection;

/** Pluggable cost estimator injected at `createFsMemory` time. */
export interface ThreadPricingCalculator {
  calculateCost(args: {
    modelId: string;
    inputTokens: number;
    outputTokens: number;
  }): number;
}

/** Options for `createThread`. All fields are optional; sensible defaults are applied. */
export interface CreateThreadParams {
  /** Provide a deterministic id instead of a random UUID. */
  id?: string;
  userId: string;
  title?: string;
  modelSelection: ModelSelection;
  /** Override the creation timestamp (ISO-8601). Useful in tests. */
  createdAt?: string;
}

export interface UpdateThreadSettingsParams {
  title?: string;
  modelSelection?: ModelSelection | null;
  now?: string;
}

export interface ReplaceThreadMessagesParams {
  messages: ThreadMessageRecord[];
  now?: string;
}

/** Payload required to append a new message to an existing thread. */
export interface SaveThreadMessageParams {
  /** Provide a deterministic message id instead of a random UUID. */
  id?: string;
  /** Persisted verbatim as a UI-layer message; convert to model messages at call time. */
  message: UIMessage;
  /** Used by the pricing calculator and to track what model was used for the message. */
  modelSelection: ModelSelection;
  usage: {
    inputTokens: number;
    outputTokens: number;
    totalDurationMs: number;
    /** If omitted, cost is calculated via `ThreadPricingCalculator` (or 0 if none). */
    totalCostUsd?: number;
  };
  error?: string | null;
  createdAt?: string;
  updatedAt?: string;
}

/**
 * Defines when the compactor should trigger.
 * Compaction appends a summary record to `history` and advances
 * `lastCompactionOrdinal` once the token budget would be exceeded.
 */
export interface CompactionPolicy {
  /** Hard limit on combined input + output tokens in the active context. */
  maxContextTokens: number;
  /**
   * Tokens to reserve for the model's next response.
   * Compaction triggers when `used + reservedOutputTokens > maxContextTokens`.
   */
  reservedOutputTokens?: number;
}

/** Options passed to `FsMemory.compactIfNeeded`. */
export interface CompactThreadOptions {
  policy: CompactionPolicy;
  /**
   * Caller-supplied function that distils the active message history into a
   * single summary `UIMessage`. The compactor appends this as a synthetic
   * record in `history` and advances `lastCompactionOrdinal`.
   */
  summarizeHistory: (history: ThreadMessageRecord[]) => Promise<UIMessage>;
  /** Override the current timestamp (ISO-8601). Useful in tests. */
  now?: string;
}

/** Returned by `FsMemory.compactIfNeeded` to indicate what happened. */
export interface CompactThreadResult {
  /** `true` if the policy threshold was crossed and a compaction occurred. */
  didCompact: boolean;
  thread: ThreadRecord;
}

/**
 * Public interface for the filesystem-backed agent memory store.
 * All mutating methods serialise through an internal write queue to
 * prevent concurrent writes from corrupting thread files.
 */
export interface AgentMemory {
  createThread(params: CreateThreadParams): Promise<ThreadRecord>;
  getThread(threadId: string): Promise<ThreadRecordTrimmed | null>;
  getThreadWithMessages(threadId: string): Promise<ThreadRecord | null>;
  listThreads(params?: {
    includeDeleted?: boolean;
    userId?: string;
  }): Promise<ThreadRecordTrimmed[]>;
  upsertMessage(
    threadId: string,
    payload: SaveThreadMessageParams,
  ): Promise<ThreadRecord | null>;
  replaceMessages(
    threadId: string,
    params: ReplaceThreadMessagesParams,
  ): Promise<ThreadRecord | null>;
  softDeleteThread(
    threadId: string,
    options?: { deletedAt?: string },
  ): Promise<boolean>;
  rebuildActiveHistory(
    threadId: string,
    options?: { now?: string },
  ): Promise<ThreadRecord | null>;
  compactIfNeeded(
    threadId: string,
    options: CompactThreadOptions,
  ): Promise<CompactThreadResult | null>;
  updateThreadSettings(
    threadId: string,
    params: UpdateThreadSettingsParams,
  ): Promise<ThreadRecord | null>;
  /** Recomputes cumulative usage totals from the full history. */
  updateThreadUsage(threadId: string): Promise<ThreadRecord | null>;
  /** Recomputes the active context-window token counts. */
  updateContextSize(threadId: string): Promise<ThreadRecord | null>;
}
