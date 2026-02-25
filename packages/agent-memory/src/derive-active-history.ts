import type { ThreadMessageRecord, ThreadRecord } from "./types";

/**
 * Derives the active context window from a thread's history.
 *
 * Messages with `ordinal > lastCompactionOrdinal` (or all messages when
 * no compaction has occurred) that have not been soft-deleted are returned
 * in ordinal order.
 */
export function deriveActiveHistory(
  thread: Pick<ThreadRecord, "history" | "lastCompactionOrdinal">,
): ThreadMessageRecord[] {
  const boundary = thread.lastCompactionOrdinal ?? 0;
  return thread.history
    .filter((m) => m.ordinal > boundary && m.deletedAt === null)
    .sort((a, b) => a.ordinal - b.ordinal);
}
