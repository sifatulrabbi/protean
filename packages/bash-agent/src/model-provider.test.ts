import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { getDefaultModelSelection } from "@protean/model-catalog";

import { createModelFromSelection } from "./model-provider";

describe("createModelFromSelection", () => {
  const originalApiKey = process.env.OPENROUTER_API_KEY;
  const originalSiteName = process.env.OPENROUTER_SITE_NAME;
  const originalSiteUrl = process.env.OPENROUTER_SITE_URL;

  beforeEach(() => {
    process.env.OPENROUTER_API_KEY = "test-openrouter-key";
    process.env.OPENROUTER_SITE_NAME = "bash-agent-tests";
    process.env.OPENROUTER_SITE_URL = "http://localhost:9999";
  });

  afterEach(() => {
    if (typeof originalApiKey === "string") {
      process.env.OPENROUTER_API_KEY = originalApiKey;
    } else {
      delete process.env.OPENROUTER_API_KEY;
    }

    if (typeof originalSiteName === "string") {
      process.env.OPENROUTER_SITE_NAME = originalSiteName;
    } else {
      delete process.env.OPENROUTER_SITE_NAME;
    }

    if (typeof originalSiteUrl === "string") {
      process.env.OPENROUTER_SITE_URL = originalSiteUrl;
    } else {
      delete process.env.OPENROUTER_SITE_URL;
    }
  });

  test("builds an OpenRouter model for a valid selection", () => {
    const selection = getDefaultModelSelection();

    const result = createModelFromSelection(selection);

    expect(result.model).toBeDefined();
    expect(result.modelInfo.providerId).toBe(selection.providerId);
    expect(result.modelInfo.id).toBe(selection.modelId);
  });

  test("rejects unsupported runtime providers", () => {
    const selection = {
      ...getDefaultModelSelection(),
      runtimeProvider: "openai",
    };

    expect(() => createModelFromSelection(selection)).toThrow(
      /Unsupported runtime provider/,
    );
  });

  test("rejects missing model entries", () => {
    const selection = {
      ...getDefaultModelSelection(),
      modelId: "missing-model-id",
    };

    expect(() => createModelFromSelection(selection)).toThrow(
      /Model entry not found/,
    );
  });

  test("rejects when OPENROUTER_API_KEY is missing", () => {
    delete process.env.OPENROUTER_API_KEY;

    expect(() => createModelFromSelection(getDefaultModelSelection())).toThrow(
      /OPENROUTER_API_KEY/,
    );
  });
});
