import {
  convertToModelMessages,
  type LanguageModel,
  type Tool,
  type ToolLoopAgent,
  type UIMessage,
} from "ai";
import {
  deriveActiveHistory,
  type AgentMemory,
  type ThreadRecord,
  type ThreadUsage,
} from "@protean/agent-memory";
import { consoleLogger, type Logger } from "@protean/logger";
import { findModel } from "@protean/model-catalog";

import { createAgent } from "./base-agent";
import { createBashTools } from "./bash-tools";
import { createFsTools } from "./fs-tools";
import { createModelFromSelection } from "./model-provider";
import { buildBashAgentPrompt } from "./prompt";

export interface BashAgentOptions {
  threadId: string;
  memory: AgentMemory;
  workspaceRoot: string;
  cwd?: string;
  instructions?: string;
  maxSteps?: number;
  bashTimeoutMs?: number;
  maxOutputBytes?: number;
  allowEnvKeys?: string[];
  modelOverride?: LanguageModel;
}

export interface BashAgentRunResult {
  thread: ThreadRecord;
  responseMessage: UIMessage;
  text: string;
  usage: ThreadUsage;
}

export interface BashAgentInstance {
  threadId: string;
  thread: ThreadRecord;
  agent: ToolLoopAgent;
  tools: Record<string, Tool>;
  generateThread(): Promise<BashAgentRunResult>;
  asTool(): Tool;
}

function summarizeHistory(thread: ThreadRecord): UIMessage {
  const summaryText = deriveActiveHistory(thread)
    .map((record) => {
      const parts = record.message.parts as Array<{
        type: string;
        text?: string;
      }>;

      return parts
        .filter((part) => part.type === "text")
        .map((part) => part.text ?? "")
        .join(" ")
        .trim();
    })
    .filter(Boolean)
    .join("\n")
    .slice(-4_000);

  return {
    id: `summary-${Date.now()}`,
    role: "user",
    parts: [
      {
        type: "text",
        text: summaryText || "Conversation summary.",
      },
    ],
  };
}

function buildAssistantMessage(text: string): UIMessage {
  return {
    id: `assistant-${Date.now()}`,
    role: "assistant",
    parts: [
      {
        type: "text",
        text: text || "",
      },
    ],
  };
}

function normalizeUsage(
  rawUsage: unknown,
  totalDurationMs: number,
): ThreadUsage {
  const usage = (rawUsage ?? {}) as Partial<ThreadUsage>;

  return {
    inputTokens: usage.inputTokens ?? 0,
    outputTokens: usage.outputTokens ?? 0,
    totalDurationMs,
    totalCostUsd: usage.totalCostUsd ?? 0,
  };
}

export async function createBashAgent(
  opts: BashAgentOptions,
  logger: Logger = consoleLogger,
): Promise<BashAgentInstance> {
  const loadedThread = await opts.memory.getThreadWithMessages(opts.threadId);

  if (!loadedThread) {
    throw new Error(`Thread "${opts.threadId}" not found.`);
  }

  let thread: ThreadRecord = loadedThread;

  const maybeModelInfo = findModel(
    thread.modelSelection.providerId,
    thread.modelSelection.modelId,
  );
  if (!maybeModelInfo) {
    throw new Error(
      `Model entry not found for ${thread.modelSelection.providerId}:${thread.modelSelection.modelId}.`,
    );
  }
  const resolvedModelInfo = maybeModelInfo;

  const model =
    opts.modelOverride ?? createModelFromSelection(thread.modelSelection).model;

  const fsTools = await createFsTools(
    {
      workspaceRoot: opts.workspaceRoot,
      cwd: opts.cwd,
      maxReadBytes: opts.maxOutputBytes,
    },
    logger,
  );
  const bashTools = await createBashTools(
    {
      workspaceRoot: opts.workspaceRoot,
      cwd: opts.cwd,
      timeoutMs: opts.bashTimeoutMs,
      maxOutputBytes: opts.maxOutputBytes,
      allowEnvKeys: opts.allowEnvKeys,
    },
    logger,
  );
  const tools = {
    ...fsTools,
    ...bashTools,
  };

  const agentWrapper = createAgent({
    name: "bash-agent",
    model,
    tools,
    instructions: opts.instructions ?? buildBashAgentPrompt(opts.workspaceRoot),
    maxSteps: opts.maxSteps,
  });

  async function generateThread(): Promise<BashAgentRunResult> {
    const compactionResult = await opts.memory.compactIfNeeded(opts.threadId, {
      policy: {
        maxContextTokens: resolvedModelInfo.contextLimits.total,
        reservedOutputTokens: resolvedModelInfo.contextLimits.maxOutput,
      },
      summarizeHistory: async () => summarizeHistory(thread),
    });

    if (compactionResult) {
      thread = compactionResult.thread;
    } else {
      const reloaded = await opts.memory.getThreadWithMessages(opts.threadId);
      if (reloaded) {
        thread = reloaded;
      }
    }

    const activeHistory = deriveActiveHistory(thread).map(
      (record) => record.message,
    );
    const startedAt = Date.now();
    const result = await agentWrapper.generate({
      messages: await convertToModelMessages(activeHistory),
    });
    const totalDurationMs = Math.max(Date.now() - startedAt, 0);
    const responseMessage = buildAssistantMessage(result.text);
    const usage = normalizeUsage(
      "usage" in result ? result.usage : undefined,
      totalDurationMs,
    );

    const updatedThread = await opts.memory.upsertMessage(opts.threadId, {
      message: responseMessage,
      modelSelection: thread.modelSelection,
      usage,
    });

    if (!updatedThread) {
      throw new Error(
        `Thread "${opts.threadId}" disappeared during persistence.`,
      );
    }

    thread = updatedThread;

    return {
      thread,
      responseMessage,
      text: result.text,
      usage,
    };
  }

  return {
    threadId: opts.threadId,
    thread,
    agent: agentWrapper.agent,
    tools,
    generateThread,
    asTool: agentWrapper.asTool,
  };
}
