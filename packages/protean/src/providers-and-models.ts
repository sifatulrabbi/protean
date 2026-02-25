import assert from "node:assert";
import type { LanguageModel } from "ai";
import {
  createOpenRouter,
  OpenRouterChatSettings,
} from "@openrouter/ai-sdk-provider";
import {
  findModel,
  type AIModelEntry,
  type ModelSelection,
} from "@protean/model-catalog";
import type { Logger } from "@protean/logger";

export function createModelFromSelection(
  modelSelection: ModelSelection,
  logger: Logger,
): { model: LanguageModel; modelInfo: AIModelEntry } {
  const fullModelEntry = findModel(
    modelSelection.providerId,
    modelSelection.modelId,
  );

  if (!fullModelEntry) {
    throw new Error("Model entry not found");
  }

  if (modelSelection.runtimeProvider !== fullModelEntry.runtimeProvider) {
    logger.warn("Model selection runtime provider mismatch.", {
      modelId: modelSelection.modelId,
      providerId: modelSelection.providerId,
      runtimeProviderFromCatalog: fullModelEntry.runtimeProvider,
      runtimeProviderFromSelection: modelSelection.runtimeProvider,
    });
  }

  if (modelSelection.runtimeProvider === "openrouter") {
    assert(process.env.OPENROUTER_API_KEY, "OPENROUTER_API_KEY is needed.");

    const openrouter = createOpenRouter({
      apiKey: process.env.OPENROUTER_API_KEY,
      headers: {
        "HTTP-Referer":
          process.env.OPENROUTER_SITE_URL || "http://localhost:3004",
        "X-Title": process.env.OPENROUTER_SITE_NAME || "protean-chatapp",
      },
    });

    logger.debug("Model config:", modelSelection);

    const chatSettings: OpenRouterChatSettings = {
      provider: {
        sort: "throughput",
        allow_fallbacks: true,
      },
    };

    if (modelSelection.reasoningBudget !== "none") {
      chatSettings.reasoning = {
        enabled: true,
        effort: modelSelection.reasoningBudget as
          | "high"
          | "medium"
          | "low"
          | "none",
      };
    }

    return {
      model: openrouter(modelSelection.modelId, chatSettings),
      modelInfo: fullModelEntry,
    };
  }

  throw new Error(
    `Unsupported runtime provider: ${modelSelection.runtimeProvider}`,
  );
}
