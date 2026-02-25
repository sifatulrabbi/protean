import { z } from "zod";
import { tavilyExtract, tavilySearch } from "@tavily/ai-sdk";
import { tool } from "ai";
import { evaluate as mathEvaluate } from "mathjs";
import { Daytona } from "@daytonaio/sdk";

const CodePython = tool({
  description: "Execute python code in an isolated ephemeral sandbox.",
  inputSchema: z.object({
    code: z.string().describe("The Python code to be executed."),
    timeout: z
      .number()
      .nullable()
      .default(120)
      .describe("Seconds to wait for the code to finish execution."),
  }),
  execute: async (args) => {
    const daytona = new Daytona({
      apiKey: process.env.DAYTONA_API_KEY,
      _experimental: {},
    });

    const sandbox = await daytona.create({
      language: "python",
    });

    const response = await sandbox.process.codeRun(args.code);

    if (response.exitCode !== 0) {
      console.error(`Error: ${response.exitCode} ${response.result}`);
    } else {
      console.log(response.result);
    }

    await sandbox.delete();
    return {
      stdout: response.result,
      exitCode: response.exitCode,
    };
  },
});

const WebSearchGeneral = tavilySearch({
  searchDepth: "basic",
  includeAnswer: true,
  maxResults: 10,
  topic: "general",
});

const WebSearchNews = tavilySearch({
  searchDepth: "basic",
  includeAnswer: true,
  maxResults: 10,
  topic: "news",
});

const WebFetchUrlContent = tavilyExtract({
  extractDepth: "basic",
  format: "markdown",
});

const Math = tool({
  description:
    "Solve complex and hard math using this tool. This tool solves multiple math expressions at once using Math.js.",
  inputSchema: z.object({
    expressions: z
      .array(z.string())
      .describe("An array of expressions to solve"),
  }),
  execute: async (args) => {
    const results: Record<string, string> = {};
    args.expressions.forEach((ex) => {
      try {
        results[ex] = mathEvaluate(ex);
      } catch (err) {
        console.error(err);
        console.error(`Math.js failed to solve ${ex}`);
        results[ex] = "Failed to solve this using Math.js";
      }
    });
    return results;
  },
});

export const Tools = {
  WebSearchGeneral,
  WebSearchNews,
  WebFetchUrlContent,
  Math,
  CodePython,
};
