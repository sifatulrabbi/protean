import { z } from "zod";

export const isoDateTimeSchema = z.string().datetime({ offset: true });

export const idSchema = z.string().min(1);

export const modelSelectionSchema = z.object({
  modelId: z.string().min(1),
  inferenceProvider: z.string().min(1),
});

export type ModelSelection = z.infer<typeof modelSelectionSchema>;

export const entityTypeSchema = z.enum(["file", "directory", "symlink"]);

export const entitySchema = z.object({
  name: z.string().min(1),
  fullPath: z.string().min(1),
  type: entityTypeSchema,
  size: z.number().int().nonnegative(),
  modifiedAt: isoDateTimeSchema.nullable(),
});

export type Entity = z.infer<typeof entitySchema>;

export const fileRangeSchema = z.object({
  startLine: z.number().int().nonnegative().default(0),
  endLine: z.number().int().nonnegative().optional(),
});

export type FileRange = z.infer<typeof fileRangeSchema>;

export const uiMessageRoleSchema = z.enum(["system", "user", "assistant", "tool"]);

export const uiTextPartSchema = z.object({
  type: z.literal("text"),
  text: z.string(),
});

export const uiReasoningPartSchema = z.object({
  type: z.literal("reasoning"),
  text: z.string(),
});

export const uiToolInvocationPartSchema = z.object({
  type: z.literal("tool-invocation"),
  toolInvocation: z.object({
    toolCallId: z.string(),
    toolName: z.string(),
    args: z.record(z.string(), z.unknown()).optional(),
    result: z.unknown().optional(),
    state: z.enum(["call", "result", "partial-call"]).default("call"),
  }),
});

export const uiMessagePartSchema = z.union([
  uiTextPartSchema,
  uiReasoningPartSchema,
  uiToolInvocationPartSchema,
]);

export type UIMessagePart = z.infer<typeof uiMessagePartSchema>;

export const threadBaseSchema = z.object({
  id: idSchema,
  userId: idSchema,
  title: z.string().min(1),
  createdAt: isoDateTimeSchema,
  updatedAt: isoDateTimeSchema,
  deletedAt: isoDateTimeSchema.nullable(),
  modelId: z.string().min(1),
  inferenceProvider: z.string().min(1),
});

export type ThreadBase = z.infer<typeof threadBaseSchema>;

export const threadUsageSchema = z.object({
  id: idSchema,
  threadId: idSchema,
  createdAt: isoDateTimeSchema,
  updatedAt: isoDateTimeSchema,
  deletedAt: isoDateTimeSchema.nullable(),
  inputTokens: z.number().int(),
  outputTokens: z.number().int(),
  durationSeconds: z.number().nonnegative(),
});

export type ThreadUsage = z.infer<typeof threadUsageSchema>;

export const threadMessageSchema = z.object({
  id: idSchema,
  threadId: idSchema,
  createdAt: isoDateTimeSchema,
  updatedAt: isoDateTimeSchema,
  deletedAt: isoDateTimeSchema.nullable(),
  role: uiMessageRoleSchema,
  parts: z.array(uiMessagePartSchema),
  metadata: z.record(z.string(), z.unknown()).nullable(),
  modelId: z.string().min(1),
  inferenceProvider: z.string().min(1),
});

export type ThreadMessage = z.infer<typeof threadMessageSchema>;

export const threadSummarySchema = threadBaseSchema.extend({
  usage: threadUsageSchema,
});

export type ThreadSummary = z.infer<typeof threadSummarySchema>;

export const threadSchema = threadSummarySchema.extend({
  messages: z.array(threadMessageSchema),
});

export type Thread = z.infer<typeof threadSchema>;

export const threadCreateSchema = z.object({
  userId: idSchema,
  title: z.string().min(1).default("New thread"),
  modelId: z.string().min(1),
  inferenceProvider: z.string().min(1),
});

export type ThreadCreate = z.infer<typeof threadCreateSchema>;

export const threadUpdateSchema = z.object({
  id: idSchema,
  userId: idSchema,
  title: z.string().min(1).optional(),
  deletedAt: isoDateTimeSchema.nullable().optional(),
  modelId: z.string().min(1).optional(),
  inferenceProvider: z.string().min(1).optional(),
});

export type ThreadUpdate = z.infer<typeof threadUpdateSchema>;

export const threadUsageUpdateSchema = z.object({
  userId: idSchema,
  threadId: idSchema,
  newInputTokens: z.number().int().default(0),
  newOutputTokens: z.number().int().default(0),
  newDurationSeconds: z.number().default(0),
});

export type ThreadUsageUpdate = z.infer<typeof threadUsageUpdateSchema>;

export const messageUpsertSchema = z.object({
  userId: idSchema,
  threadId: idSchema,
  messageId: idSchema.optional(),
  role: uiMessageRoleSchema,
  parts: z.array(uiMessagePartSchema),
  metadata: z.record(z.string(), z.unknown()).nullable().optional(),
  modelId: z.string().min(1),
  inferenceProvider: z.string().min(1),
});

export type MessageUpsert = z.infer<typeof messageUpsertSchema>;

export const messageDeleteSchema = z.object({
  userId: idSchema,
  threadId: idSchema,
  messageId: idSchema,
});

export type MessageDelete = z.infer<typeof messageDeleteSchema>;

export const sandboxExecRequestSchema = z.object({
  cmd: z.string().min(1),
  scope: z.string().optional(),
});

export const sandboxExecResponseSchema = z.object({
  result: z.object({
    stdout: z.string(),
    stderr: z.string(),
  }),
  error: z.string().nullable(),
});

export type SandboxExecRequest = z.infer<typeof sandboxExecRequestSchema>;
export type SandboxExecResponse = z.infer<typeof sandboxExecResponseSchema>;

export const sandboxReadFileRequestSchema = z.object({
  fullPath: z.string().min(1),
  range: fileRangeSchema.default({ startLine: 0 }),
});

export const sandboxReadFileResponseSchema = z.object({
  fullPath: z.string(),
  content: z.string(),
  range: fileRangeSchema,
  error: z.string().nullable(),
});

export type SandboxReadFileResponse = z.infer<typeof sandboxReadFileResponseSchema>;

export const sandboxListDirResponseSchema = z.object({
  fullPath: z.string(),
  entries: z.array(entitySchema),
  error: z.string().nullable(),
});

export type SandboxListDirResponse = z.infer<typeof sandboxListDirResponseSchema>;

export const sandboxStatResponseSchema = z.object({
  fullPath: z.string(),
  entity: entitySchema.nullable(),
  error: z.string().nullable(),
});

export type SandboxStatResponse = z.infer<typeof sandboxStatResponseSchema>;

export const sandboxWriteFileRequestSchema = z.object({
  fullPath: z.string().min(1),
  content: z.string(),
});

export const sandboxMutationResponseSchema = z.object({
  fullPath: z.string(),
  error: z.string().nullable(),
});

export type SandboxMutationResponse = z.infer<typeof sandboxMutationResponseSchema>;

export const sandboxCreateRequestSchema = z.object({
  fullPath: z.string().min(1),
  entityType: z.enum(["file", "directory"]),
  opts: z.object({
    recursive: z.boolean().optional(),
  }).optional(),
});

export const sandboxCreateResponseSchema = z.object({
  fullPath: z.string(),
  entity: entitySchema,
  error: z.string().nullable(),
});

export type SandboxCreateResponse = z.infer<typeof sandboxCreateResponseSchema>;

export const sandboxCopyRequestSchema = z.object({
  sourceFullPath: z.string().min(1),
  copyToFullPath: z.string().min(1),
});

export const sandboxCopyResponseSchema = z.object({
  sourceFullPath: z.string(),
  copyToFullPath: z.string(),
  error: z.string().nullable(),
});

export type SandboxCopyResponse = z.infer<typeof sandboxCopyResponseSchema>;

export const sandboxMoveRequestSchema = z.object({
  sourceFullPath: z.string().min(1),
  newFullPath: z.string().min(1),
  opts: z.object({
    recursive: z.boolean().optional(),
  }).optional(),
});

export const sandboxMoveResponseSchema = z.object({
  sourceFullPath: z.string(),
  newFullPath: z.string(),
  error: z.string().nullable(),
});

export type SandboxMoveResponse = z.infer<typeof sandboxMoveResponseSchema>;

export const sandboxRemoveRequestSchema = z.object({
  fullPath: z.string().min(1),
  opts: z.object({
    recursive: z.boolean().optional(),
  }).optional(),
});

export const sandboxProjectSchema = z.object({
  name: z.string().min(1),
  fullPath: z.string().min(1),
  source: z.enum(["remote", "mount"]),
  mountId: z.string().nullable(),
});

export type SandboxProject = z.infer<typeof sandboxProjectSchema>;

export const mountSchema = z.object({
  id: idSchema,
  workspacePath: z.string().min(1),
  sourcePath: z.string().min(1),
  projectName: z.string().min(1),
  createdAt: isoDateTimeSchema,
});

export type Mount = z.infer<typeof mountSchema>;

export const chatRequestSchema = z.object({
  threadId: z.string().min(1).optional(),
  userId: z.string().min(1),
  projectName: z.string().min(1),
  message: z.string().min(1),
  selectedSkills: z.array(z.string()).default([]),
  modelId: z.string().min(1),
  inferenceProvider: z.string().min(1),
});

export type ChatRequest = z.infer<typeof chatRequestSchema>;

export const chatResponseSchema = z.object({
  thread: threadSchema,
  reply: z.string(),
});

export type ChatResponse = z.infer<typeof chatResponseSchema>;

export const todoItemSchema = z.object({
  id: z.string().min(1),
  content: z.string().min(1),
  status: z.enum(["pending", "in_progress", "completed"]),
});

export type TodoItem = z.infer<typeof todoItemSchema>;

export const skillDescriptorSchema = z.object({
  name: z.string().min(1),
  description: z.string().min(1),
});

export type SkillDescriptor = z.infer<typeof skillDescriptorSchema>;

export const sandboxHealthSchema = z.object({
  ok: z.boolean(),
  workspaceRoot: z.string(),
  projectsRoot: z.string(),
  mountsRoot: z.string(),
});

export type SandboxHealth = z.infer<typeof sandboxHealthSchema>;

export interface ILogger {
  info: (message: string, metadata?: Record<string, unknown>) => void;
  warn: (message: string, metadata?: Record<string, unknown>) => void;
  error: (message: string, metadata?: Record<string, unknown>) => void;
}

export interface IBash {
  exec: (
    cmd: string,
    scope?: string,
  ) => Promise<{
    result: { stdout: string; stderr: string };
    error: Error | null;
  }>;
}

export interface IFilesystem {
  readFile: (
    fullPath: string,
    range?: FileRange,
  ) => Promise<{
    fullPath: string;
    content: string;
    range: FileRange;
    error: Error | null;
  }>;
  listDir: (
    fullPath: string,
  ) => Promise<{ fullPath: string; entries: Entity[]; error: Error | null }>;
  stat: (fullPath: string) => Promise<{
    fullPath: string;
    entity: Entity | null;
    error: Error | null;
  }>;
  writeFile: (
    fullPath: string,
    content: string,
  ) => Promise<{ fullPath: string; error: Error | null }>;
  create: (
    fullPath: string,
    entityType: "file" | "directory",
    opts?: { recursive?: boolean },
  ) => Promise<{
    fullPath: string;
    entity: Entity;
    error: Error | null;
  }>;
  copy: (
    sourceFullPath: string,
    copyToFullPath: string,
  ) => Promise<{
    sourceFullPath: string;
    copyToFullPath: string;
    error: Error | null;
  }>;
  move: (
    sourceFullPath: string,
    newFullPath: string,
    opts?: { recursive?: boolean },
  ) => Promise<{
    sourceFullPath: string;
    newFullPath: string;
    error: Error | null;
  }>;
  remove: (
    fullPath: string,
    opts?: { recursive?: boolean },
  ) => Promise<{
    fullPath: string;
    error: Error | null;
  }>;
}

export interface ISandboxClient {
  isConnected: boolean;
  connErr: Error | null;
  bash: IBash;
  fs: IFilesystem;
}

export interface IMount extends IFilesystem {
  id: string;
  workspacePath: string;
}

export interface IProject {
  fs: IFilesystem;
  bash: IBash;
}

export interface IAgentMemory {
  getThreads: (userId: string) => Promise<ThreadSummary[]>;
  getThread: (payload: { id: string; userId: string }) => Promise<ThreadSummary>;
  getThreadWithMessages: (payload: { id: string; userId: string }) => Promise<Thread>;
  createThread: (
    payload: Omit<ThreadBase, "id" | "createdAt" | "updatedAt"> & { userId: string },
  ) => Promise<ThreadSummary>;
  updateThread: (payload: Partial<ThreadBase> & { id: string; userId: string }) => Promise<ThreadSummary>;
  updateThreadUsage: (payload: ThreadUsageUpdate) => Promise<ThreadUsage>;
  deleteThread: (payload: { id: string; userId: string }) => Promise<void>;
  upsertMessage: (payload: MessageUpsert) => Promise<Thread>;
  deleteMessage: (payload: MessageDelete) => Promise<Thread>;
}
