import assert from "node:assert";
import type { LanguageModel } from "ai";
import {
  createOpenRouter,
  type OpenRouterChatSettings,
} from "@openrouter/ai-sdk-provider";
import {
  findModel,
  type AIModelEntry,
  type ModelSelection,
} from "@protean/model-catalog";

export function createModelFromSelection(modelSelection: ModelSelection): {
  model: LanguageModel;
  modelInfo: AIModelEntry;
} {
  const modelInfo = findModel(
    modelSelection.providerId,
    modelSelection.modelId,
  );

  if (!modelInfo) {
    throw new Error(
      `Model entry not found for ${modelSelection.providerId}:${modelSelection.modelId}.`,
    );
  }

  if (modelSelection.runtimeProvider !== "openrouter") {
    throw new Error(
      `Unsupported runtime provider: ${modelSelection.runtimeProvider}.`,
    );
  }

  assert(process.env.OPENROUTER_API_KEY, "OPENROUTER_API_KEY is needed.");

  const openrouter = createOpenRouter({
    apiKey: process.env.OPENROUTER_API_KEY,
    headers: {
      "HTTP-Referer":
        process.env.OPENROUTER_SITE_URL ?? "http://localhost:3004",
      "X-Title": process.env.OPENROUTER_SITE_NAME ?? "protean-bash-agent",
    },
  });

  const chatSettings: OpenRouterChatSettings = {
    provider: {
      sort: "throughput",
      allow_fallbacks: true,
    },
  };

  if (modelSelection.reasoningBudget !== "none") {
    chatSettings.reasoning = {
      enabled: true,
      effort: modelSelection.reasoningBudget,
    };
  }

  return {
    model: openrouter(modelSelection.modelId, chatSettings),
    modelInfo,
  };
}
