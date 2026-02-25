import { createOpenAI } from "@ai-sdk/openai";
import { createOpenRouter } from "@openrouter/ai-sdk-provider";

const openaiProvider = createOpenAI({
  apiKey: process.env.OPENAI_API_KEY,
});

const openRouterProvider = createOpenRouter({
  apiKey: process.env.OPENROUTER_API_KEY,
  headers: {
    "HTTP-Referer": process.env.OPENROUTER_SITE_URL || "http://localhost:3004",
    "X-Title": process.env.OPENROUTER_SITE_NAME || "chatapp-mvp",
  },
});

function getProviderFromId(providerId: string) {
  switch (providerId) {
    case "openai":
      if (!process.env.OPENAI_API_KEY) {
        throw new Error("OPENAI_API_KEY is not provided.");
      }
      return openaiProvider;
    case "openrouter":
      if (!process.env.OPENROUTER_API_KEY) {
        throw new Error("OPENROUTER_API_KEY is not provided.");
      }
      return openRouterProvider;
    default:
      throw new Error(`Invalid provider ${providerId}`);
  }
}

export const ModelProviders = {
  openaiProvider,
  openRouterProvider,
  getProviderFromId,
};
