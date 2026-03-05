import { afterEach, describe, expect, test } from "bun:test";

import { createWebTools } from "./web-tools";

describe("createWebTools", () => {
  const originalTavilyKey = process.env.TAVILY_API_KEY;

  afterEach(() => {
    if (typeof originalTavilyKey === "string") {
      process.env.TAVILY_API_KEY = originalTavilyKey;
    } else {
      delete process.env.TAVILY_API_KEY;
    }
  });

  test("returns empty tools when Tavily key is missing", () => {
    delete process.env.TAVILY_API_KEY;

    expect(createWebTools()).toEqual({});
  });

  test("returns empty tools when explicitly disabled", () => {
    process.env.TAVILY_API_KEY = "test-key";

    expect(createWebTools({ enabled: false })).toEqual({});
  });

  test("returns legacy named web tools when enabled", () => {
    process.env.TAVILY_API_KEY = "test-key";

    const tools = createWebTools({ enabled: true });
    expect(Object.keys(tools).sort()).toEqual([
      "WebFetchUrlContent",
      "WebSearchGeneral",
      "WebSearchNews",
    ]);
  });
});
