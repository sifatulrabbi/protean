import { describe, expect, it } from "bun:test";
import {
  parseModelSelection,
  resolveModelSelection,
  isSameModelSelection,
} from "../model-selection";
import { getDefaultModelSelection } from "../catalog-parser";

describe("parseModelSelection", () => {
  it("parses valid input", () => {
    const result = parseModelSelection({
      providerId: "openrouter",
      modelId: "test/model",
      reasoningBudget: "none",
      runtimeProvider: "openrouter",
    });
    expect(result).toEqual({
      providerId: "openrouter",
      modelId: "test/model",
      reasoningBudget: "none",
      runtimeProvider: "openrouter",
    });
  });

  it("returns undefined for invalid input", () => {
    expect(parseModelSelection(null)).toBeUndefined();
    expect(parseModelSelection({})).toBeUndefined();
    expect(parseModelSelection({ providerId: "" })).toBeUndefined();
    expect(
      parseModelSelection({
        providerId: "x",
        modelId: "y",
        reasoningBudget: "invalid",
        runtimeProvider: "openrouter",
      }),
    ).toBeUndefined();
  });

  it("returns undefined for missing fields", () => {
    expect(
      parseModelSelection({ providerId: "x", modelId: "y" }),
    ).toBeUndefined();
  });
});

describe("resolveModelSelection", () => {
  it("falls back to system default when no candidates given", () => {
    const result = resolveModelSelection({});
    const defaultSelection = getDefaultModelSelection();
    expect(result).toEqual(defaultSelection);
  });

  it("uses request override when model exists in catalog", () => {
    const defaultSelection = getDefaultModelSelection();
    const result = resolveModelSelection({
      request: defaultSelection,
    });
    expect(result.modelId).toBe(defaultSelection.modelId);
  });

  it("uses thread selection when request is invalid", () => {
    const defaultSelection = getDefaultModelSelection();
    const result = resolveModelSelection({
      request: { providerId: "bad", modelId: "bad", reasoningBudget: "none" },
      thread: defaultSelection,
    });
    expect(result.modelId).toBe(defaultSelection.modelId);
  });

  it("falls back to default when both request and thread are invalid", () => {
    const result = resolveModelSelection({
      request: { providerId: "bad", modelId: "bad", reasoningBudget: "none" },
      thread: {
        providerId: "bad",
        modelId: "bad2",
        reasoningBudget: "none",
        runtimeProvider: "openrouter",
      },
    });
    const defaultSelection = getDefaultModelSelection();
    expect(result).toEqual(defaultSelection);
  });
});

describe("isSameModelSelection", () => {
  it("returns true for identical selections", () => {
    const selection = getDefaultModelSelection();
    expect(isSameModelSelection(selection, selection)).toBe(true);
  });

  it("returns false when first is undefined", () => {
    const selection = getDefaultModelSelection();
    expect(isSameModelSelection(undefined, selection)).toBe(false);
  });

  it("returns false for different models", () => {
    const selection = getDefaultModelSelection();
    expect(
      isSameModelSelection({ ...selection, modelId: "different" }, selection),
    ).toBe(false);
  });

  it("returns false for different reasoning budgets", () => {
    const selection = getDefaultModelSelection();
    expect(
      isSameModelSelection(
        { ...selection, reasoningBudget: "high" },
        { ...selection, reasoningBudget: "low" },
      ),
    ).toBe(false);
  });
});
