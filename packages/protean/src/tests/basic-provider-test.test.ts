import { createOpenRouter } from "@openrouter/ai-sdk-provider";
import { ToolLoopAgent, stepCountIs, tool } from "ai";
import z from "zod";

const weatherByCity: Record<string, { condition: string; tempC: number }> = {
  "new york": { condition: "Cloudy", tempC: 6 },
  "london": { condition: "Rain", tempC: 8 },
  "san francisco": { condition: "Fog", tempC: 13 },
  "tokyo": { condition: "Clear", tempC: 11 },
};

const GetCurrentTime = tool({
  description: "Get the current local time for a given IANA timezone.",
  inputSchema: z.object({
    timeZone: z
      .string()
      .default("UTC")
      .describe("IANA timezone, e.g. America/New_York"),
  }),
  execute: async ({ timeZone }) => {
    try {
      const now = new Date();
      const localTime = new Intl.DateTimeFormat("en-US", {
        dateStyle: "full",
        timeStyle: "long",
        timeZone,
      }).format(now);

      console.log("[GetCurrentTime] called");
      return {
        iso: now.toISOString(),
        localTime,
        timeZone,
      };
    } catch {
      return {
        error: `Invalid timezone: ${timeZone}`,
      };
    }
  },
});

const GetWeatherReport = tool({
  description: "Get a simple weather report for a city.",
  inputSchema: z.object({
    city: z.string().min(1),
    unit: z.enum(["C", "F"]).default("C"),
  }),
  execute: async ({ city, unit }) => {
    const normalizedCity = city.trim().toLowerCase();
    const weather = weatherByCity[normalizedCity] ?? {
      condition: "Unknown",
      tempC: 20,
    };
    const temperature =
      unit === "F" ? Math.round((weather.tempC * 9) / 5 + 32) : weather.tempC;

    console.log("[GetWeatherReport] called");
    return {
      city,
      condition: weather.condition,
      temperature,
      unit,
    };
  },
});

async function main(): Promise<void> {
  const provider = createOpenRouter({
    apiKey: process.env.OPENROUTER_API_KEY,
    headers: {
      "HTTP-Referer":
        process.env.OPENROUTER_SITE_URL || "http://localhost:3004",
      "X-Title":
        process.env.OPENROUTER_SITE_NAME || "protean-basic-provider-test",
    },
  });

  const agent = new ToolLoopAgent({
    model: provider("google/gemini-3-flash-preview", {
      reasoning: {
        effort: "medium",
      },
    }),
    instructions: "You are a helpful assistant.",
    tools: {
      GetCurrentTime,
      GetWeatherReport,
    },
    activeTools: ["GetCurrentTime", "GetWeatherReport"],
    toolChoice: "auto",
    stopWhen: stepCountIs(10),
    onStepFinish: (ctx) => {
      console.log(
        `[STEP FINISH] finishReason=${ctx.finishReason} toolCalls=${ctx.toolCalls.length} reasoning=${!!ctx.reasoningText}`,
      );
    },
  });

  const result = await agent.generate({
    messages: [
      {
        role: "user",
        content:
          "What time is it in America/New_York, and what's the weather in San Francisco in F?",
      },
    ],
  });

  console.log("text:", result.text);
}

void main().catch((error) => {
  console.error("OpenRouter basic provider test failed.");
  console.error(error);
  process.exitCode = 1;
});
