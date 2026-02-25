/**
 * The valid reasoning-effort levels supported by the system.
 * Matches the OpenRouter `reasoning.effort` parameter.
 */
export const reasoningBudgets = ["none", "low", "medium", "high"] as const;
export type ReasoningBudget = (typeof reasoningBudgets)[number];

/** Persisted model/provider selection per thread (and per-request override). */
export interface ModelSelection {
  providerId: string;
  modelId: string;
  reasoningBudget: ReasoningBudget;
  runtimeProvider: string;
}
