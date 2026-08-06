import { mkdir, readdir, readFile, stat, writeFile } from "node:fs/promises";
import path from "node:path";

import type {
  IAgentMemory,
  MessageDelete,
  MessageUpsert,
  Thread,
  ThreadBase,
  ThreadSummary,
  ThreadUsage,
  ThreadUsageUpdate,
} from "@protean/contracts";
import { threadSchema, threadSummarySchema, threadUsageSchema } from "@protean/contracts";
import { ulid } from "ulid";

type FileAgentMemoryOptions = {
  baseDir: string;
};

type StoredThread = Thread;

const DEFAULT_TITLE = "New thread";

function nowIso() {
  return new Date().toISOString();
}

function deriveTitleFromMessage(payload: MessageUpsert) {
  const firstTextPart = payload.parts.find((part) => part.type === "text");

  if (!firstTextPart) {
    return DEFAULT_TITLE;
  }

  const compact = firstTextPart.text.trim().replace(/\s+/g, " ");

  if (!compact) {
    return DEFAULT_TITLE;
  }

  return compact.slice(0, 48);
}

async function ensureDir(dir: string) {
  await mkdir(dir, { recursive: true });
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

export class FileAgentMemory implements IAgentMemory {
  constructor(private readonly options: FileAgentMemoryOptions) {}

  private get userRootDir() {
    return path.join(this.options.baseDir, "users");
  }

  private getThreadPath(userId: string, threadId: string) {
    return path.join(this.userRootDir, userId, "threads", `${threadId}.json`);
  }

  private async writeThread(thread: StoredThread) {
    const filePath = this.getThreadPath(thread.userId, thread.id);

    await ensureDir(path.dirname(filePath));
    await writeFile(filePath, JSON.stringify(thread, null, 2));
  }

  private async listUserThreadPaths(userId: string) {
    const threadsDir = path.join(this.userRootDir, userId, "threads");

    try {
      const entries = await readdir(threadsDir);
      return entries.filter((entry) => entry.endsWith(".json")).map((entry) => path.join(threadsDir, entry));
    } catch {
      return [];
    }
  }

  private async readThread(userId: string, threadId: string) {
    const filePath = this.getThreadPath(userId, threadId);
    const fileContent = await readFile(filePath, "utf8");
    return threadSchema.parse(JSON.parse(fileContent));
  }

  private async loadThreadSummaries(userId: string) {
    const threadPaths = await this.listUserThreadPaths(userId);
    const threads = await Promise.all(
      threadPaths.map(async (threadPath) => {
        const raw = await readFile(threadPath, "utf8");
        return threadSchema.parse(JSON.parse(raw));
      }),
    );

    return threads
      .filter((thread) => thread.deletedAt === null)
      .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))
      .map((thread) => threadSummarySchema.parse({
        ...thread,
        usage: thread.usage,
      }));
  }

  async getThreads(userId: string): Promise<ThreadSummary[]> {
    return this.loadThreadSummaries(userId);
  }

  async getThread(payload: { id: string; userId: string }): Promise<ThreadSummary> {
    const thread = await this.readThread(payload.userId, payload.id);
    return threadSummarySchema.parse({
      ...thread,
      usage: thread.usage,
    });
  }

  async getThreadWithMessages(payload: { id: string; userId: string }): Promise<Thread> {
    return this.readThread(payload.userId, payload.id);
  }

  async createThread(
    payload: Omit<ThreadBase, "id" | "createdAt" | "updatedAt"> & { userId: string },
  ): Promise<ThreadSummary> {
    const timestamp = nowIso();
    const threadId = ulid();
    const usage: ThreadUsage = threadUsageSchema.parse({
      id: ulid(),
      threadId,
      createdAt: timestamp,
      updatedAt: timestamp,
      deletedAt: null,
      inputTokens: 0,
      outputTokens: 0,
      durationSeconds: 0,
    });
    const thread = threadSchema.parse({
      id: threadId,
      userId: payload.userId,
      title: payload.title || DEFAULT_TITLE,
      createdAt: timestamp,
      updatedAt: timestamp,
      deletedAt: payload.deletedAt ?? null,
      modelId: payload.modelId,
      inferenceProvider: payload.inferenceProvider,
      usage,
      messages: [],
    });

    await this.writeThread(thread);

    return this.getThread({ id: thread.id, userId: thread.userId });
  }

  async updateThread(payload: Partial<ThreadBase> & { id: string; userId: string }): Promise<ThreadSummary> {
    const thread = await this.readThread(payload.userId, payload.id);
    const nextThread = threadSchema.parse({
      ...thread,
      ...payload,
      updatedAt: nowIso(),
    });

    await this.writeThread(nextThread);

    return this.getThread({ id: nextThread.id, userId: nextThread.userId });
  }

  async updateThreadUsage(payload: ThreadUsageUpdate): Promise<ThreadUsage> {
    const thread = await this.readThread(payload.userId, payload.threadId);
    const nextUsage = threadUsageSchema.parse({
      ...thread.usage,
      updatedAt: nowIso(),
      inputTokens: thread.usage.inputTokens + payload.newInputTokens,
      outputTokens: thread.usage.outputTokens + payload.newOutputTokens,
      durationSeconds: Math.max(0, thread.usage.durationSeconds + payload.newDurationSeconds),
    });

    await this.writeThread(
      threadSchema.parse({
        ...thread,
        updatedAt: nowIso(),
        usage: nextUsage,
      }),
    );

    return nextUsage;
  }

  async deleteThread(payload: { id: string; userId: string }): Promise<void> {
    const thread = await this.readThread(payload.userId, payload.id);

    await this.writeThread(
      threadSchema.parse({
        ...thread,
        updatedAt: nowIso(),
        deletedAt: nowIso(),
      }),
    );
  }

  async upsertMessage(payload: MessageUpsert): Promise<Thread> {
    const thread = await this.readThread(payload.userId, payload.threadId);
    const timestamp = nowIso();
    const existingIndex = payload.messageId
      ? thread.messages.findIndex((message) => message.id === payload.messageId)
      : -1;
    const messageId = payload.messageId ?? ulid();
    const nextMessage = {
      id: messageId,
      threadId: payload.threadId,
      createdAt: existingIndex >= 0 ? thread.messages[existingIndex].createdAt : timestamp,
      updatedAt: timestamp,
      deletedAt: null,
      role: payload.role,
      parts: payload.parts,
      metadata: payload.metadata ?? null,
      modelId: payload.modelId,
      inferenceProvider: payload.inferenceProvider,
    } as const;
    const nextMessages = clone(thread.messages);

    if (existingIndex >= 0) {
      nextMessages[existingIndex] = nextMessage;
    } else {
      nextMessages.push(nextMessage);
    }

    const nextThread = threadSchema.parse({
      ...thread,
      title:
        thread.title === DEFAULT_TITLE && payload.role === "user"
          ? deriveTitleFromMessage(payload)
          : thread.title,
      updatedAt: timestamp,
      messages: nextMessages,
    });

    await this.writeThread(nextThread);

    return nextThread;
  }

  async deleteMessage(payload: MessageDelete): Promise<Thread> {
    const thread = await this.readThread(payload.userId, payload.threadId);
    const timestamp = nowIso();
    const nextThread = threadSchema.parse({
      ...thread,
      updatedAt: timestamp,
      messages: thread.messages.map((message) =>
        message.id === payload.messageId
          ? {
              ...message,
              deletedAt: timestamp,
              updatedAt: timestamp,
            }
          : message,
      ),
    });

    await this.writeThread(nextThread);

    return nextThread;
  }
}

export async function createFileAgentMemory(options: FileAgentMemoryOptions) {
  await ensureDir(path.join(options.baseDir, "users"));
  return new FileAgentMemory(options);
}

export async function agentMemoryExists(options: FileAgentMemoryOptions, userId: string, threadId: string) {
  try {
    const fileStats = await stat(path.join(options.baseDir, "users", userId, "threads", `${threadId}.json`));
    return fileStats.isFile();
  } catch {
    return false;
  }
}
