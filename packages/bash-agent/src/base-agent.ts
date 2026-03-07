import {
  stepCountIs,
  tool,
  type LanguageModel,
  type Tool,
  ToolLoopAgent,
} from "ai";
import { z } from "zod";

export interface AgentOptions {
  name: string;
  model: LanguageModel;
  tools: Record<string, Tool>;
  instructions: string;
  maxSteps?: number;
}

export function createAgent(opts: AgentOptions) {
  const agent = new ToolLoopAgent({
    id: opts.name,
    model: opts.model,
    instructions: opts.instructions,
    toolChoice: "auto",
    tools: opts.tools,
    stopWhen: stepCountIs(Math.max(1, opts.maxSteps ?? 50)),
  });

  const asTool = () =>
    tool({
      description: `Delegate a focused task to the "${opts.name}" agent.`,
      inputSchema: z.object({
        prompt: z.string().min(1).describe("Task for the sub-agent."),
      }),
      execute: async ({ prompt }) => {
        const result = await agent.generate({
          messages: [{ role: "user", content: prompt }],
        });

        return {
          text: result.text,
          finishReason:
            "finishReason" in result ? String(result.finishReason) : "unknown",
        };
      },
    });

  return {
    agent,
    generate: agent.generate.bind(agent),
    stream: agent.stream.bind(agent),
    asTool,
  };
}
