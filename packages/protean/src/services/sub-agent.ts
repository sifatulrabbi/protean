import { tool, type LanguageModel, type Tool } from "ai";
import type { Skill } from "@protean/skill";
import type { Logger } from "@protean/logger";

import { createSkillOrchestrator } from "../skill-orchestrator";
import { buildSubAgentSystemPrompt } from "../prompts/sub-agent-prompt";
import z from "zod";

export type OutputStrategy = "string" | "workspace-file" | "tmp-file";

const MAX_SPAWN_DEPTH = 3;

export interface SubAgentDependencies {
  model: LanguageModel;
  skillsList: Skill<unknown>[];
  baseTools?: { [k: string]: Tool };
}

function extractOutputPathFromSteps(
  steps: Array<{
    staticToolCalls: Array<{ toolName: string; input: unknown }>;
  }>,
): string | undefined {
  for (let i = steps.length - 1; i >= 0; i--) {
    const step = steps[i];
    for (let j = step.staticToolCalls.length - 1; j >= 0; j--) {
      const toolCall = step.staticToolCalls[j];
      if (toolCall.toolName !== "WriteFile") {
        continue;
      }

      const input = toolCall.input;
      if (
        typeof input === "object" &&
        input !== null &&
        "path" in input &&
        typeof (input as { path: unknown }).path === "string"
      ) {
        return (input as { path: string }).path;
      }
    }
  }

  return undefined;
}

const spawnSubAgentInputSchema = z.object({
  skillIds: z
    .array(z.string())
    .describe("Which skills the sub-agent should have access to."),
  goal: z
    .string()
    .describe("The focused task for the sub-agent to accomplish."),
  systemPrompt: z
    .string()
    .optional()
    .describe("Optional custom system prompt override for the sub-agent."),
  outputStrategy: z
    .enum(["string", "workspace-file", "tmp-file"])
    .describe("How the sub-agent should return its output."),
});

/**
 * Creates the SpawnSubAgent tool with depth-tracked nesting support.
 *
 * Each nesting level gets a new tool instance that captures the current depth,
 * so depth is tracked structurally rather than relying on LLM input.
 * Sub-agents can spawn their own sub-agents up to MAX_SPAWN_DEPTH levels deep.
 */
export function createSubAgentTools(
  deps: SubAgentDependencies,
  logger: Logger,
  currentDepth = 0,
): { SpawnSubAgent: Tool } {
  const { model, skillsList, baseTools = {} } = deps;
  let spawnCount = 0;

  const SpawnSubAgent: Tool = tool({
    description:
      "Launch a focused sub-agent with a specific set of skills to accomplish a goal. Sub-agents run independently and return their output when finished.",
    inputSchema: spawnSubAgentInputSchema,
    execute: async (args) => {
      const depth = currentDepth + 1;
      const subAgentId = ++spawnCount;

      if (depth > MAX_SPAWN_DEPTH) {
        logger.warn(
          `[SubAgent #${subAgentId}] Spawn rejected — max depth ${MAX_SPAWN_DEPTH} exceeded`,
          { depth, goal: args.goal.slice(0, 200) },
        );
        return {
          status: "error",
          error: `Sub-agent nesting depth limit (${MAX_SPAWN_DEPTH}) reached. Complete this task directly instead of spawning another sub-agent.`,
        };
      }

      logger.info(`[SubAgent #${subAgentId}] Spawning`, {
        depth,
        skillIds: args.skillIds,
        outputStrategy: args.outputStrategy,
        goalPreview: args.goal.slice(0, 200),
        hasCustomPrompt: !!args.systemPrompt,
      });

      // Create a child SpawnSubAgent tool at the next depth level
      const childSubAgentTools = createSubAgentTools(
        { model, skillsList, baseTools },
        logger,
        depth,
      );
      const childBaseTools = { ...baseTools, ...childSubAgentTools };

      const agent = await createSkillOrchestrator(
        {
          model,
          instructionsBuilder: buildSubAgentSystemPrompt(
            args.outputStrategy,
            args.systemPrompt,
          ),
          skillsList,
          baseTools: childBaseTools,
        },
        logger,
      );

      const result = await agent.generate({
        messages: [
          {
            role: "user",
            content: `${args.goal}\n\n_Use your skills to complete the tasks._`,
          },
        ],
      });

      const outputPath = extractOutputPathFromSteps(result.staticToolCalls);

      logger.info(`[SubAgent #${subAgentId}] Completed`, {
        depth,
        steps: result.staticToolCalls.length,
        outputLength: result.text.length,
        outputPath: outputPath ?? null,
      });

      return {
        status: "done",
        output: result.text,
        outputPath,
      };
    },
  });

  return { SpawnSubAgent };
}
