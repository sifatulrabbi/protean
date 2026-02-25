import { describe, expect, it } from "bun:test";
import { modelSelectionSchema } from "../schema";

describe("modelSelectionSchema", () => {
  it("validates a correct model selection", () => {
    const result = modelSelectionSchema.safeParse({
      providerId: "openrouter",
      modelId: "anthropic/claude-3.5-sonnet",
      reasoningBudget: "medium",
      runtimeProvider: "openrouter",
    });
    expect(result.success).toBe(true);
  });

  it("validates all reasoning budget values", () => {
    for (const budget of ["none", "low", "medium", "high"] as const) {
      const result = modelSelectionSchema.safeParse({
        providerId: "openrouter",
        modelId: "test/model",
        reasoningBudget: budget,
        runtimeProvider: "openrouter",
      });
      expect(result.success).toBe(true);
    }
  });

  it("rejects empty providerId", () => {
    const result = modelSelectionSchema.safeParse({
      providerId: "",
      modelId: "test/model",
      reasoningBudget: "none",
      runtimeProvider: "openrouter",
    });
    expect(result.success).toBe(false);
  });

  it("rejects empty modelId", () => {
    const result = modelSelectionSchema.safeParse({
      providerId: "openrouter",
      modelId: "",
      reasoningBudget: "none",
      runtimeProvider: "openrouter",
    });
    expect(result.success).toBe(false);
  });

  it("rejects invalid reasoning budget", () => {
    const result = modelSelectionSchema.safeParse({
      providerId: "openrouter",
      modelId: "test/model",
      reasoningBudget: "extreme",
      runtimeProvider: "openrouter",
    });
    expect(result.success).toBe(false);
  });

  it("rejects missing fields", () => {
    const result = modelSelectionSchema.safeParse({
      providerId: "openrouter",
    });
    expect(result.success).toBe(false);
  });

  it("rejects null input", () => {
    const result = modelSelectionSchema.safeParse(null);
    expect(result.success).toBe(false);
  });
});
