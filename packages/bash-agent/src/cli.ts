import { randomUUID } from "node:crypto";
import { createInterface } from "node:readline/promises";
import { config } from "dotenv";
import { createFsMemory } from "@protean/agent-memory";
import { noopLogger } from "@protean/logger";
import { getDefaultModelSelection } from "@protean/model-catalog";
import {
  DEFAULT_SANDBOX_PROJECT_NAME,
  createSandboxClient,
} from "@protean/sandbox-client";

import { createBashAgent } from "./bash-agent";

config();

function buildUserMessage(text: string) {
  return {
    id: `user-${randomUUID()}`,
    role: "user" as const,
    parts: [
      {
        type: "text" as const,
        text,
      },
    ],
  };
}

function requireEnv(key: string): string {
  const value = process.env[key]?.trim();
  if (!value) {
    throw new Error(`Missing required environment variable: ${key}`);
  }

  return value;
}

function isExitCommand(prompt: string): boolean {
  const normalized = prompt.trim().toLowerCase();
  return normalized === "/exit" || normalized === "/quit";
}

async function readStdinPrompt(): Promise<string> {
  let input = "";
  for await (const chunk of process.stdin) {
    input += chunk;
  }

  return input.trim();
}

async function runTurn(input: {
  prompt: string;
  threadId: string;
  memory: Awaited<ReturnType<typeof createFsMemory>>;
  modelSelection: ReturnType<typeof getDefaultModelSelection>;
  agent: Awaited<ReturnType<typeof createBashAgent>>;
}): Promise<string> {
  await input.memory.upsertMessage(input.threadId, {
    message: buildUserMessage(input.prompt),
    modelSelection: input.modelSelection,
    usage: {
      inputTokens: 0,
      outputTokens: 0,
      totalDurationMs: 0,
      totalCostUsd: 0,
    },
  });

  const result = await input.agent.generateThread();
  return result.text;
}

async function runInteractiveChat(input: {
  initialPrompt?: string;
  threadId: string;
  memory: Awaited<ReturnType<typeof createFsMemory>>;
  modelSelection: ReturnType<typeof getDefaultModelSelection>;
  agent: Awaited<ReturnType<typeof createBashAgent>>;
}): Promise<void> {
  const rl = createInterface({
    input: process.stdin,
    output: process.stdout,
  });

  let pendingPrompt = input.initialPrompt?.trim() || undefined;
  process.stdout.write('Bash Agent Chat CLI. Type "/exit" to quit.\n\n');

  try {
    while (true) {
      const nextInput =
        pendingPrompt ?? (await rl.question("You: ").catch(() => null));
      pendingPrompt = undefined;

      if (nextInput === null) {
        break;
      }

      const prompt = nextInput.trim();
      if (!prompt) {
        continue;
      }

      if (isExitCommand(prompt)) {
        break;
      }

      try {
        const reply = await runTurn({
          prompt,
          threadId: input.threadId,
          memory: input.memory,
          modelSelection: input.modelSelection,
          agent: input.agent,
        });
        process.stdout.write(`Assistant:\n${reply}\n\n`);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        process.stderr.write(`Error: ${message}\n\n`);
      }
    }
  } finally {
    rl.close();
  }
}

async function main(): Promise<void> {
  const initialPrompt = process.argv.slice(2).join(" ").trim() || undefined;

  const serviceBaseUrl = requireEnv("SANDBOX_BASE_URL");
  const serviceToken = requireEnv("SANDBOX_SERVICE_TOKEN");
  const sessionId = requireEnv("SANDBOX_SESSION_ID");
  const projectName =
    process.env.SANDBOX_PROJECT_NAME?.trim() || DEFAULT_SANDBOX_PROJECT_NAME;
  const sandboxClient = await createSandboxClient({
    baseUrl: serviceBaseUrl,
    serviceToken,
    sessionId,
    logger: noopLogger,
  });

  const memory = await createFsMemory(
    { fs: sandboxClient.fs, dirPath: ".threads" },
    noopLogger,
  );
  const modelSelection = getDefaultModelSelection();
  const thread = await memory.createThread({
    userId: "bash-agent-cli",
    title: "Bash Agent Chat CLI",
    projectName,
    modelSelection,
  });

  const agent = await createBashAgent(
    {
      threadId: thread.id,
      memory,
      environment: {
        kind: "sandbox",
        serviceBaseUrl,
        serviceToken,
        sessionId,
        projectName,
      },
    },
    noopLogger,
  );

  if (!process.stdin.isTTY) {
    const prompt = initialPrompt ?? (await readStdinPrompt());
    if (!prompt) {
      throw new Error("No prompt provided.");
    }

    const result = await runTurn({
      prompt,
      threadId: thread.id,
      memory,
      modelSelection,
      agent,
    });
    process.stdout.write(`${result}\n`);
    return;
  }

  await runInteractiveChat({
    initialPrompt,
    threadId: thread.id,
    memory,
    modelSelection,
    agent,
  });
}

main().catch((error) => {
  const message = error instanceof Error ? error.message : String(error);
  process.stderr.write(`${message}\n`);
  process.exitCode = 1;
});
