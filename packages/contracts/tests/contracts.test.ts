import { describe, expect, it } from "bun:test";

import { chatRequestSchema, threadSchema } from "../src";

describe("@protean/contracts", () => {
  it("validates thread payloads", () => {
    const thread = threadSchema.parse({
      id: "thread_1",
      userId: "user_1",
      title: "Protean thread",
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
      deletedAt: null,
      modelId: "gpt-4o-mini",
      inferenceProvider: "openrouter",
      usage: {
        id: "usage_1",
        threadId: "thread_1",
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        deletedAt: null,
        inputTokens: 0,
        outputTokens: 0,
        durationSeconds: 0,
      },
      messages: [],
    });

    expect(thread.id).toBe("thread_1");
  });

  it("validates chat requests", () => {
    const request = chatRequestSchema.parse({
      userId: "user_1",
      projectName: "protean",
      message: "Inspect the repo",
      modelId: "gpt-4o-mini",
      inferenceProvider: "openrouter",
    });

    expect(request.selectedSkills).toEqual([]);
  });
});
