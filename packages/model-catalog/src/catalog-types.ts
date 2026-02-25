export interface AIModelEntry {
  id: string;
  name: string;
  providerId: string;
  runtimeProvider: string;
  features: {
    reasoning: boolean;
    tools: boolean;
  };
  reasoning: {
    budgets: string[];
    defaultValue: string;
  };
  contextLimits: {
    total: number;
    maxInput: number;
    maxOutput: number;
  };
  pricing: {
    inputUsdPerMillion: number | null;
    outputUsdPerMillion: number | null;
  };
}

export interface AIModelProviderEntry {
  id: string;
  name: string;
  models: AIModelEntry[];
}
