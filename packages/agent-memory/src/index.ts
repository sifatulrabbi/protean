/**
 * agent-memory — filesystem-backed conversation thread persistence.
 *
 * Primary entry point:
 * ```ts
 * import { createFsMemory } from "@your-scope/agent-memory";
 * const memory = await createFsMemory({ fs: yourFsAdapter }, logger);
 * ```
 */
export { ThreadMemoryError } from "./errors";
export { createHistoryCompactor } from "./compaction";
export {
  createFsMemory,
  modelSelectionSchema,
  deriveActiveHistory,
} from "./fs-memory";
export type { FsMemoryOptions } from "./fs-memory";

export { reasoningBudgets } from "./types";
export type {
  AgentMemory,
  ThreadMessageRecord,
  ThreadRecord,
  ThreadRecordTrimmed,
  ThreadUsage,
  ContextSize,
  ModelSelection,
  ReasoningBudget,
  /** @deprecated Use ModelSelection instead. */
  ThreadModelSelection,
  ThreadPricingCalculator,
  CompactionPolicy,
  CompactThreadOptions,
  CompactThreadResult,
  CreateThreadParams,
  ReplaceThreadMessagesParams,
  SaveThreadMessageParams,
  UpdateThreadSettingsParams,
} from "./types";
