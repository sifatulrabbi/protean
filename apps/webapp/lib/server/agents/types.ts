export interface PromptVarsDefault {
  verbosity?: string;
  personality?: string;
}

export interface AgentFactoryConfig {
  reasoningBudget?: string;
  outputVerbosity?: string;
  instructions?: PromptVarsDefault;
}
