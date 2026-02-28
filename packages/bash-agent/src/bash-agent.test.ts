import { afterEach, beforeEach, describe, expect, mock, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import type { LanguageModel, UIMessage } from "ai";
import {
  createFsMemory,
  deriveActiveHistory,
  type AgentMemory,
  type ThreadRecord,
} from "@protean/agent-memory";
import { noopLogger } from "@protean/logger";
import { findModel, getDefaultModelSelection } from "@protean/model-catalog";
import { createLocalFs } from "@protean/vfs";

let generateCalls: Array<{ messages: unknown[] }> = [];

await mock.module("./base-agent", () => ({
  createAgent: () => ({
    agent: { id: "mock-bash-agent" },
    generate: async (args: { messages: unknown[] }) => {
      generateCalls.push(args);
      return {
        text: "assistant reply",
        usage: {
          inputTokens: 12,
          outputTokens: 4,
        },
      };
    },
    stream: async () => {
      throw new Error("stream not implemented in tests");
    },
    asTool: () => ({
      description: "mock",
      inputSchema: undefined,
      execute: async () => ({ text: "assistant reply" }),
    }),
  }),
}));

const { createBashAgent } = await import("./bash-agent");

function buildMessage(role: "user" | "assistant", text: string): UIMessage {
  return {
    id: `${role}-${Math.random().toString(36).slice(2)}`,
    role,
    parts: [
      {
        type: "text",
        text,
      },
    ],
  };
}

async function createMemoryFixture(): Promise<{
  workspaceRoot: string;
  memory: AgentMemory;
}> {
  const workspaceRoot = await mkdtemp(join(tmpdir(), "bash-agent-runtime-"));
  const fs = await createLocalFs(workspaceRoot, noopLogger);
  const memory = await createFsMemory({ fs }, noopLogger);
  return {
    workspaceRoot,
    memory,
  };
}

describe("createBashAgent", () => {
  let workspaceRoot = "";
  let memory: AgentMemory;
  let thread: ThreadRecord;
  const originalApiKey = process.env.OPENROUTER_API_KEY;

  beforeEach(async () => {
    generateCalls = [];
    const fixture = await createMemoryFixture();
    workspaceRoot = fixture.workspaceRoot;
    memory = fixture.memory;
    thread = await memory.createThread({
      userId: "user-1",
      title: "Bash Agent Thread",
      modelSelection: getDefaultModelSelection(),
    });
    process.env.OPENROUTER_API_KEY = "unused-test-key";
  });

  afterEach(async () => {
    mock.clearAllMocks();

    if (workspaceRoot) {
      await rm(workspaceRoot, { recursive: true, force: true });
    }

    if (typeof originalApiKey === "string") {
      process.env.OPENROUTER_API_KEY = originalApiKey;
    } else {
      delete process.env.OPENROUTER_API_KEY;
    }
  });

  test("loads model selection from the thread record", async () => {
    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        workspaceRoot,
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    expect(agent.thread.modelSelection).toEqual(thread.modelSelection);
  });

  test("uses active history rather than raw history when generating", async () => {
    const first = await memory.upsertMessage(thread.id, {
      message: buildMessage("user", "hidden old content"),
      modelSelection: thread.modelSelection,
      usage: {
        inputTokens: 100,
        outputTokens: 0,
        totalDurationMs: 1,
      },
    });
    expect(first).not.toBeNull();

    const compacted = await memory.compactIfNeeded(thread.id, {
      policy: {
        maxContextTokens: 1,
        reservedOutputTokens: 0,
      },
      summarizeHistory: async () => buildMessage("user", "summary only"),
    });
    expect(compacted?.didCompact).toBe(true);

    const second = await memory.upsertMessage(thread.id, {
      message: buildMessage("user", "visible message"),
      modelSelection: thread.modelSelection,
      usage: {
        inputTokens: 1,
        outputTokens: 0,
        totalDurationMs: 1,
      },
    });
    expect(second).not.toBeNull();

    const reloaded = await memory.getThreadWithMessages(thread.id);
    expect(reloaded).not.toBeNull();
    expect(deriveActiveHistory(reloaded!).length).toBeLessThan(
      reloaded!.history.length,
    );

    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        workspaceRoot,
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    await agent.generateThread();

    const serialized = JSON.stringify(generateCalls.at(-1)?.messages ?? []);
    expect(serialized).not.toContain("hidden old content");
    expect(serialized).toContain("visible message");
  });

  test("compacts before generation when context limits are exceeded", async () => {
    const modelInfo = findModel(
      thread.modelSelection.providerId,
      thread.modelSelection.modelId,
    );
    expect(modelInfo).toBeDefined();

    const saved = await memory.upsertMessage(thread.id, {
      message: buildMessage(
        "user",
        "token ".repeat(
          (modelInfo?.contextLimits.total ?? 1) +
            (modelInfo?.contextLimits.maxOutput ?? 0) +
            1_000,
        ),
      ),
      modelSelection: thread.modelSelection,
      usage: {
        inputTokens: 1,
        outputTokens: 0,
        totalDurationMs: 1,
      },
    });
    expect(saved).not.toBeNull();

    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        workspaceRoot,
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    const result = await agent.generateThread();

    expect(result.thread.lastCompactionOrdinal).not.toBeNull();
  });

  test("persists the assistant reply after generateThread", async () => {
    const saved = await memory.upsertMessage(thread.id, {
      message: buildMessage("user", "hello"),
      modelSelection: thread.modelSelection,
      usage: {
        inputTokens: 1,
        outputTokens: 0,
        totalDurationMs: 1,
      },
    });
    expect(saved).not.toBeNull();

    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        workspaceRoot,
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    const result = await agent.generateThread();
    const reloaded = await memory.getThreadWithMessages(thread.id);

    expect(result.text).toBe("assistant reply");
    expect(reloaded?.history.at(-1)?.message.role).toBe("assistant");
    expect(
      JSON.stringify(reloaded?.history.at(-1)?.message.parts ?? []),
    ).toContain("assistant reply");
  });

  test("does not require OPENROUTER_API_KEY when modelOverride is provided", async () => {
    delete process.env.OPENROUTER_API_KEY;

    const result = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        workspaceRoot,
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    expect(result).toBeDefined();
  });
});
