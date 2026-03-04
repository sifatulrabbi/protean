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
import { createSandboxClient } from "@protean/sandbox-client";

import { createAgent } from "./base-agent";
import {
  createBashTools,
  shellQuote,
  type ShellRunner,
  type ShellRunnerInput,
} from "./bash-tools";
import { createFsTools, type Globber } from "./fs-tools";
import { createModelFromSelection } from "./model-provider";
import { buildBashAgentPrompt } from "./prompt";
import {
  resolveWithinWorkspace,
  toWorkspaceRelativePath,
} from "./workspace-paths";

export interface LocalBashEnvironment {
  kind: "local";
  workspaceRoot: string;
  allowEnvKeys?: string[];
}

export interface SandboxBashEnvironment {
  kind: "sandbox";
  serviceBaseUrl: string;
  serviceToken: string;
  sessionId: string;
  workspaceRoot?: string;
}

export interface BashAgentOptions {
  threadId: string;
  memory: AgentMemory;
  environment: LocalBashEnvironment | SandboxBashEnvironment;
  cwd?: string;
  instructions?: string;
  maxSteps?: number;
  bashTimeoutMs?: number;
  maxOutputBytes?: number;
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

function buildSandboxGlobber(
  workspaceRoot: string,
  runner: ShellRunner,
  timeoutMs: number,
  maxOutputBytes: number,
): Globber {
  const pythonScript = `import glob, os, sys

pattern = sys.argv[1]
include_dirs = sys.argv[2] == "1"

limit = int(sys.argv[3])
seen = set()
count = 0

for match in glob.iglob(pattern, recursive=True):
    normalized = match.replace("\\\\", "/")

    if not include_dirs and os.path.isdir(match):
        continue

    if normalized in seen:
        continue

    print(normalized)
    seen.add(normalized)
    count += 1

    if count >= limit:
        break
`;

  return {
    glob: async ({ pattern, cwd, includeDirectories, maxResults }) => {
      const result = await runner.exec({
        command: [
          "python3",
          "-c",
          shellQuote(pythonScript),
          shellQuote(pattern),
          includeDirectories ? "1" : "0",
          String(maxResults),
        ].join(" "),
        cwd,
        timeoutMs,
        maxOutputBytes,
      });

      if (result.exitCode !== 0) {
        const stderr = result.stderr.trim();
        throw new Error(stderr || "Sandbox glob failed.");
      }

      const cwdAbsolute = resolveWithinWorkspace(workspaceRoot, cwd);
      return result.stdout
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean)
        .map((line) =>
          toWorkspaceRelativePath(
            workspaceRoot,
            resolveWithinWorkspace(cwdAbsolute, line),
          ),
        );
    },
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

  const defaultTimeoutMs = opts.bashTimeoutMs ?? 30_000;
  const maxOutputBytes = opts.maxOutputBytes ?? 64 * 1024;
  let shellRunner: ShellRunner | undefined;
  let globber: Globber | undefined;
  let fsTools: Record<string, Tool>;
  let bashTools: Record<string, Tool>;
  let workspaceRoot: string;

  if (opts.environment.kind === "sandbox") {
    const sandboxClient = await createSandboxClient({
      baseUrl: opts.environment.serviceBaseUrl,
      serviceToken: opts.environment.serviceToken,
      sessionId: opts.environment.sessionId,
      logger,
    });
    workspaceRoot =
      opts.environment.workspaceRoot ?? sandboxClient.workspaceMountPath;

    shellRunner = {
      exec: async ({ command, cwd, timeoutMs }: ShellRunnerInput) =>
        sandboxClient.exec({
          command,
          cwd,
          timeoutMs,
        }),
    };

    globber = buildSandboxGlobber(
      workspaceRoot,
      shellRunner,
      defaultTimeoutMs,
      maxOutputBytes,
    );

    fsTools = await createFsTools(
      {
        workspaceRoot,
        fs: sandboxClient.fs,
        cwd: opts.cwd,
        maxReadBytes: opts.maxOutputBytes,
        globber,
      },
      logger,
    );
    bashTools = await createBashTools(
      {
        workspaceRoot,
        cwd: opts.cwd,
        timeoutMs: opts.bashTimeoutMs,
        maxOutputBytes: opts.maxOutputBytes,
        runner: shellRunner,
      },
      logger,
    );
  } else {
    workspaceRoot = opts.environment.workspaceRoot;
    fsTools = await createFsTools(
      {
        workspaceRoot,
        cwd: opts.cwd,
        maxReadBytes: opts.maxOutputBytes,
      },
      logger,
    );
    bashTools = await createBashTools(
      {
        workspaceRoot,
        cwd: opts.cwd,
        timeoutMs: opts.bashTimeoutMs,
        maxOutputBytes: opts.maxOutputBytes,
        allowEnvKeys: opts.environment.allowEnvKeys,
      },
      logger,
    );
  }

  const tools = {
    ...fsTools,
    ...bashTools,
  };

  const agentWrapper = createAgent({
    name: "bash-agent",
    model,
    tools,
    instructions: opts.instructions ?? buildBashAgentPrompt(workspaceRoot),
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
