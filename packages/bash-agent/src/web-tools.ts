import type { Tool } from "ai";
import { tavilyExtract, tavilySearch } from "@tavily/ai-sdk";

interface CreateWebToolsOptions {
  enabled?: boolean;
}

function shouldEnableWebTools(enabled?: boolean): boolean {
  if (enabled === false) {
    return false;
  }

  return Boolean(process.env.TAVILY_API_KEY?.trim());
}

export function createWebTools(
  opts?: CreateWebToolsOptions,
): Record<string, Tool> {
  if (!shouldEnableWebTools(opts?.enabled)) {
    return {};
  }

  const WebSearchGeneral = tavilySearch({
    searchDepth: "basic",
    includeAnswer: true,
    maxResults: 20,
    topic: "general",
  });

  const WebSearchNews = tavilySearch({
    searchDepth: "basic",
    includeAnswer: true,
    maxResults: 20,
    topic: "news",
  });

  const WebFetchUrlContent = tavilyExtract({
    extractDepth: "basic",
    format: "markdown",
  });

  return {
    WebSearchGeneral,
    WebSearchNews,
    WebFetchUrlContent,
  };
}
