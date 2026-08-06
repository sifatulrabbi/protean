import { afterEach, describe, expect, it } from "bun:test";
import { rm } from "node:fs/promises";
import path from "node:path";

import { createFileAgentMemory } from "../src";

const testRoot = path.join(process.cwd(), "tmp", "agent-memory-tests");

afterEach(async () => {
  await rm(testRoot, { recursive: true, force: true });
});

describe("FileAgentMemory", () => {
  it("creates, updates, and loads thread state", async () => {
    const memory = await createFileAgentMemory({ baseDir: testRoot });
    const thread = await memory.createThread({
      userId: "user_1",
      title: "New thread",
      deletedAt: null,
      modelId: "gpt-5.4",
      inferenceProvider: "openrouter",
    });

    await memory.upsertMessage({
      userId: "user_1",
      threadId: thread.id,
      role: "user",
      parts: [{ type: "text", text: "Help me prepare a launch checklist for Protean." }],
      metadata: null,
      modelId: "gpt-5.4",
      inferenceProvider: "openrouter",
    });

    await memory.updateThreadUsage({
      userId: "user_1",
      threadId: thread.id,
      newInputTokens: 120,
      newOutputTokens: 340,
      newDurationSeconds: 4.2,
    });

    const loaded = await memory.getThreadWithMessages({ id: thread.id, userId: "user_1" });

    expect(loaded.title).toBe("Help me prepare a launch checklist for Protean.");
    expect(loaded.messages).toHaveLength(1);
    expect(loaded.usage.inputTokens).toBe(120);
    expect(loaded.usage.outputTokens).toBe(340);
  });

  it("soft deletes threads from thread listings", async () => {
    const memory = await createFileAgentMemory({ baseDir: testRoot });
    const thread = await memory.createThread({
      userId: "user_2",
      title: "Scratch",
      deletedAt: null,
      modelId: "gpt-5.4",
      inferenceProvider: "openai",
    });

    await memory.deleteThread({ id: thread.id, userId: "user_2" });

    const threads = await memory.getThreads("user_2");

    expect(threads).toHaveLength(0);
  });
});
