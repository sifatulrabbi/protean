import { stepCountIs, ToolLoopAgent } from "ai";
import { type AIModelEntry } from "@protean/model-catalog";

import type { AgentFactoryConfig } from "./types";
import { ModelProviders } from "./model-providers";
import { Prompts } from "./prompts";
import { Tools } from "./tools";

function buildProviderOpts(
  providerId: string,
  reasoningBudget: AgentFactoryConfig["reasoningBudget"],
) {
  if (["openai", "openrouter"].includes(providerId)) {
    return {
      [providerId]: {
        reasoningEffort: reasoningBudget,
        reasoningSummary: "auto",
      },
    };
  }

  return undefined;
}

function getAgent(modelCfg: AIModelEntry, cfg?: AgentFactoryConfig) {
  const provider = ModelProviders.getProviderFromId(modelCfg.runtimeProvider);

  const agent = new ToolLoopAgent({
    model: provider(modelCfg.id),
    providerOptions: buildProviderOpts(
      modelCfg.runtimeProvider,
      cfg?.reasoningBudget || modelCfg.reasoning.defaultValue,
    ),
    instructions: Prompts.defaultPrompt(cfg?.instructions),
    toolChoice: "auto",
    tools: Tools,
    stopWhen: stepCountIs(200),
  });

  return agent;
}

export const AgentsFactory = {
  getAgent,
};
