import { z } from "zod";
import { reasoningBudgets, type ModelSelection } from "./types";

export const modelSelectionSchema: z.ZodType<ModelSelection> = z.object({
  providerId: z.string().min(1),
  modelId: z.string().min(1),
  reasoningBudget: z.enum(reasoningBudgets),
  runtimeProvider: z.string().default("openrouter"),
});
