import type {
  ThreadRecord,
  ThreadRecordTrimmed,
  ThreadUsage,
} from "@protean/agent-memory";
import type { UIMessage } from "ai";

export function threadToUiMessages(thread: ThreadRecord): UIMessage[] {
  return thread.history
    .filter((message) => message.deletedAt === null)
    .sort((a, b) => a.ordinal - b.ordinal)
    .map((message) => message.message);
}

export function threadToMessageUsageMap(
  thread: ThreadRecord,
): Record<string, ThreadUsage> {
  const map: Record<string, ThreadUsage> = {};
  for (const record of thread.history) {
    if (record.deletedAt === null) {
      map[record.message.id] = record.usage;
    }
  }
  return map;
}

export function canAccessThread(
  thread: ThreadRecordTrimmed,
  userId: string,
): boolean {
  return thread.userId === userId && thread.deletedAt === null;
}
