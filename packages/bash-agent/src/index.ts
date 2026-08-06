import { readFile } from "node:fs/promises";
import path from "node:path";

import { createOpenAI } from "@ai-sdk/openai";
import type { IAgentMemory, ILogger, Thread } from "@protean/contracts";
import { createProject } from "@protean/sandbox-sdk";
import { generateText, stepCountIs, tool } from "ai";
import { z } from "zod";

import type { HttpSandboxClient } from "@protean/sandbox-sdk";

type AgentSkill = {
  name: string;
  description: string;
};

type RunAgentTurnOptions = {
  message: string;
  userId: string;
  projectName: string;
  modelId: string;
  inferenceProvider: string;
  threadId?: string;
  selectedSkills?: string[];
  sandboxClient: HttpSandboxClient;
  memory: IAgentMemory;
  logger?: ILogger;
  skillsFilePath?: string;
  forceFallback?: boolean;
};

type RunAgentTurnResult = {
  thread: Thread;
  reply: string;
};

const DEFAULT_MODEL = "gpt-4o-mini";
const DEFAULT_PROVIDER = "openrouter";

const noopLogger: ILogger = {
  info: () => undefined,
  warn: () => undefined,
  error: () => undefined,
};

function safeLogger(logger?: ILogger) {
  return logger ?? noopLogger;
}

function getWorkspaceRoot() {
  return process.env.PROTEAN_WORKSPACE_ROOT || path.join(process.cwd(), "tmp", "workspace");
}

async function readAvailableSkills(skillsFilePath?: string) {
  const resolvedPath = skillsFilePath ?? path.join(process.cwd(), "AGENTS.md");

  try {
    const file = await readFile(resolvedPath, "utf8");
    const matches = [...file.matchAll(/- ([a-z0-9-.]+): (.+?) \(file:/gi)];

    return matches.map(
      (match) =>
        ({
          name: match[1]!,
          description: match[2]!,
        }) satisfies AgentSkill,
    );
  } catch {
    return [];
  }
}

function createInferenceModel(modelId?: string, provider?: string) {
  const selectedProvider = provider || DEFAULT_PROVIDER;
  const selectedModel = modelId || DEFAULT_MODEL;

  if (selectedProvider === "openai" && process.env.OPENAI_API_KEY) {
    const openai = createOpenAI({ apiKey: process.env.OPENAI_API_KEY });
    return openai.chat(selectedModel);
  }

  if (process.env.OPENROUTER_API_KEY) {
    const openrouter = createOpenAI({
      apiKey: process.env.OPENROUTER_API_KEY,
      baseURL: "https://openrouter.ai/api/v1",
      name: "openrouter",
      headers: {
        "HTTP-Referer": process.env.OPENROUTER_SITE_URL || "https://protean.local",
        "X-Title": process.env.OPENROUTER_SITE_NAME || "Protean",
      },
    });
    return openrouter.chat(selectedModel);
  }

  return null;
}

function renderThreadContext(thread: Thread) {
  const visibleMessages = thread.messages.filter((message) => message.deletedAt === null);
  const earlierMessages = visibleMessages.slice(0, -10);
  const recentMessages = visibleMessages.slice(-10);
  const summary = earlierMessages.length
    ? [
        "Earlier conversation summary:",
        ...earlierMessages.slice(-6).map((message) => {
          const text = message.parts
            .filter((part): part is Extract<(typeof message.parts)[number], { type: "text" }> => part.type === "text")
            .map((part) => part.text)
            .join("\n")
            .trim();

          if (!text) {
            return `${message.role.toUpperCase()}: [non-text content omitted]`;
          }

          return `${message.role.toUpperCase()}: ${text.slice(0, 280)}`;
        }),
        "",
      ].join("\n")
    : "";

  return [summary, ...recentMessages
    .map((message) => {
      const text = message.parts
        .filter((part): part is Extract<(typeof message.parts)[number], { type: "text" }> => part.type === "text")
        .map((part) => part.text)
        .join("\n");
      return `${message.role.toUpperCase()}: ${text}`;
    })]
    .filter(Boolean)
    .join("\n\n");
}

function buildSystemPrompt(input: {
  availableSkills: AgentSkill[];
  selectedSkills: string[];
  projectName: string;
}) {
  const chosenSkills = input.availableSkills.filter((skill) => input.selectedSkills.includes(skill.name));
  const skillSection = (chosenSkills.length > 0 ? chosenSkills : input.availableSkills.slice(0, 12))
    .map((skill) => `- ${skill.name}: ${skill.description}`)
    .join("\n");

  return [
    "You are Protean, an AI agent focused on exploring a project and finishing the user's task.",
    "Use tools when they materially improve accuracy or let you modify files safely.",
    "Be concrete, action-oriented, and brief.",
    `Current project: ${input.projectName}`,
    "Available skills:",
    skillSection || "- No skills catalog was loaded.",
  ].join("\n");
}

async function runFallbackTurn(options: {
  message: string;
  thread: Thread;
  projectPath: string;
  sandboxClient: HttpSandboxClient;
}) {
  const project = createProject({
    sandboxClient: options.sandboxClient,
    projectPath: options.projectPath,
  });
  const dirListing = await project.fs.listDir(".");
  const fileNames = dirListing.entries.slice(0, 12).map((entry) => entry.name).join(", ");
  const reply = [
    "Protean is running without an LLM provider key, so this is the deterministic local fallback.",
    `Project scanned: ${options.projectPath}`,
    fileNames ? `Top-level entries: ${fileNames}` : "The project is currently empty.",
    `Latest request: ${options.message}`,
  ].join("\n");

  return reply;
}

export async function runAgentTurn(options: RunAgentTurnOptions): Promise<RunAgentTurnResult> {
  const logger = safeLogger(options.logger);
  const projectPath = `/workspace/projects/${options.projectName}`;
  const model = options.forceFallback ? null : createInferenceModel(options.modelId, options.inferenceProvider);
  const availableSkills = await readAvailableSkills(options.skillsFilePath);
  const threadSummary = options.threadId
    ? await options.memory.getThread({ id: options.threadId, userId: options.userId })
    : await options.memory.createThread({
        userId: options.userId,
        title: "New thread",
        deletedAt: null,
        modelId: options.modelId,
        inferenceProvider: options.inferenceProvider,
      });

  await options.memory.upsertMessage({
    userId: options.userId,
    threadId: threadSummary.id,
    role: "user",
    parts: [{ type: "text", text: options.message }],
    metadata: { projectName: options.projectName },
    modelId: options.modelId,
    inferenceProvider: options.inferenceProvider,
  });

  const threadWithUserMessage = await options.memory.getThreadWithMessages({
    id: threadSummary.id,
    userId: options.userId,
  });

  let reply: string;
  let inputTokens = 0;
  let outputTokens = 0;

  if (!model) {
    reply = await runFallbackTurn({
      message: options.message,
      thread: threadWithUserMessage,
      projectPath,
      sandboxClient: options.sandboxClient,
    });
  } else {
    logger.info("Running Protean agent turn.", {
      threadId: threadSummary.id,
      provider: options.inferenceProvider,
      modelId: options.modelId,
    });

    const project = createProject({
      sandboxClient: options.sandboxClient,
      projectPath,
    });

    const result = await generateText({
      model,
      system: buildSystemPrompt({
        availableSkills,
        selectedSkills: options.selectedSkills ?? [],
        projectName: options.projectName,
      }),
      prompt: [
        "Conversation so far:",
        renderThreadContext(threadWithUserMessage),
        "",
        `Workspace root: ${getWorkspaceRoot()}`,
        "Resolve the user's latest request. Prefer using tools instead of guessing about files.",
      ].join("\n"),
      tools: {
        listDirectory: tool({
          description: "List entries in a project directory.",
          inputSchema: z.object({
            fullPath: z.string().describe("Path relative to the current project."),
          }),
          execute: async ({ fullPath }) => project.fs.listDir(fullPath),
        }),
        readFile: tool({
          description: "Read a file from the current project.",
          inputSchema: z.object({
            fullPath: z.string(),
            startLine: z.number().int().nonnegative().default(0),
            endLine: z.number().int().positive().optional(),
          }),
          execute: async ({ fullPath, startLine, endLine }) =>
            project.fs.readFile(fullPath, {
              startLine,
              ...(endLine ? { endLine } : {}),
            }),
        }),
        writeFile: tool({
          description: "Write content to a file inside the current project.",
          inputSchema: z.object({
            fullPath: z.string(),
            content: z.string(),
          }),
          execute: async ({ fullPath, content }) => project.fs.writeFile(fullPath, content),
        }),
        runCommand: tool({
          description: "Run a bash command inside the current project.",
          inputSchema: z.object({
            cmd: z.string(),
            scope: z.string().optional(),
          }),
          execute: async ({ cmd, scope }) => project.bash.exec(cmd, scope),
        }),
      },
      stopWhen: stepCountIs(6),
    });

    reply = result.text;
    inputTokens = result.totalUsage?.inputTokens ?? result.usage?.inputTokens ?? 0;
    outputTokens = result.totalUsage?.outputTokens ?? result.usage?.outputTokens ?? 0;
  }

  await options.memory.upsertMessage({
    userId: options.userId,
    threadId: threadSummary.id,
    role: "assistant",
    parts: [{ type: "text", text: reply }],
    metadata: { projectName: options.projectName },
    modelId: options.modelId,
    inferenceProvider: options.inferenceProvider,
  });

  await options.memory.updateThreadUsage({
    userId: options.userId,
    threadId: threadSummary.id,
    newInputTokens: inputTokens,
    newOutputTokens: outputTokens,
    newDurationSeconds: 0,
  });

  return {
    thread: await options.memory.getThreadWithMessages({ id: threadSummary.id, userId: options.userId }),
    reply,
  };
}

export { buildSystemPrompt, createInferenceModel, readAvailableSkills };
