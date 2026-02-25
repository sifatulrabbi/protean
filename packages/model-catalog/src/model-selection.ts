import { modelSelectionSchema } from "./schema";
import { findModel, getDefaultModelSelection } from "./catalog-parser";
import type { ModelSelection } from "./types";

/**
 * Parses and validates a model selection from an unknown input.
 * Returns `undefined` if validation fails.
 */
export function parseModelSelection(
  input: unknown,
): ModelSelection | undefined {
  const parsed = modelSelectionSchema.safeParse(input);
  return parsed.success ? parsed.data : undefined;
}

/**
 * Resolves a valid model selection from a cascade of candidates:
 * request override -> thread default -> system default.
 *
 * For each candidate, validates the model exists in the catalog
 * and that the reasoning budget is supported by the model.
 */
export function resolveModelSelection(args: {
  request?: Partial<ModelSelection>;
  thread?: ModelSelection;
}): ModelSelection {
  const fallback = getDefaultModelSelection();
  const candidates: (Partial<ModelSelection> | undefined)[] = [
    args.request,
    args.thread,
    fallback,
  ];

  for (const candidate of candidates) {
    if (!candidate?.providerId || !candidate?.modelId) {
      continue;
    }

    const model = findModel(candidate.providerId, candidate.modelId);
    if (!model) {
      continue;
    }

    const requestedBudget = candidate.reasoningBudget;
    const reasoningBudget =
      requestedBudget && model.reasoning.budgets.includes(requestedBudget)
        ? requestedBudget
        : model.reasoning.defaultValue;

    return {
      providerId: candidate.providerId,
      modelId: candidate.modelId,
      reasoningBudget: reasoningBudget as ModelSelection["reasoningBudget"],
      runtimeProvider: model.runtimeProvider,
    };
  }

  return fallback;
}

export function isSameModelSelection(
  a: ModelSelection | undefined,
  b: ModelSelection,
): boolean {
  if (!a) {
    return false;
  }

  return (
    a.providerId === b.providerId &&
    a.modelId === b.modelId &&
    a.reasoningBudget === b.reasoningBudget &&
    a.runtimeProvider === b.runtimeProvider
  );
}
