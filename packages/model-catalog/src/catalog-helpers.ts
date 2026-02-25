import type { AIModelEntry, AIModelProviderEntry } from "./catalog-types";
import type { ModelSelection } from "./types";

export function getProviderById(
  providers: AIModelProviderEntry[],
  providerId: string,
): AIModelProviderEntry | undefined {
  return providers.find((provider) => provider.id === providerId);
}

export function getModelById(
  providers: AIModelProviderEntry[],
  providerId: string,
  modelId: string,
): AIModelEntry | undefined {
  const provider = getProviderById(providers, providerId);
  return provider?.models.find((model) => model.id === modelId);
}

export function getModelReasoningById(
  providers: AIModelProviderEntry[],
  providerId: string,
  modelId: string,
): AIModelEntry["reasoning"] | undefined {
  return getModelById(providers, providerId, modelId)?.reasoning;
}

/**
 * Client-side model selection resolution.
 * Validates model exists in provider catalog and budget is supported.
 */
export function resolveClientModelSelection(
  providers: AIModelProviderEntry[],
  defaultSelection: ModelSelection,
  modelSelection?: Partial<ModelSelection>,
): ModelSelection {
  const providerId = modelSelection?.providerId ?? defaultSelection.providerId;
  const modelId = modelSelection?.modelId ?? defaultSelection.modelId;

  const selectedModel = getModelById(providers, providerId, modelId);
  const fallbackModel = getModelById(
    providers,
    defaultSelection.providerId,
    defaultSelection.modelId,
  );

  const resolvedModel = selectedModel ?? fallbackModel;

  if (!resolvedModel) {
    return defaultSelection;
  }

  const requestedBudget = modelSelection?.reasoningBudget;
  const reasoningBudget =
    requestedBudget && resolvedModel.reasoning.budgets.includes(requestedBudget)
      ? requestedBudget
      : resolvedModel.reasoning.defaultValue;

  return {
    providerId: resolvedModel.providerId,
    modelId: resolvedModel.id,
    reasoningBudget: reasoningBudget as ModelSelection["reasoningBudget"],
    runtimeProvider: resolvedModel.runtimeProvider,
  };
}
