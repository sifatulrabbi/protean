import {
  tool,
  stepCountIs,
  ToolLoopAgent,
  type Tool,
  type LanguageModel,
} from "ai";
import z from "zod";
import { type Skill } from "@protean/skill";
import { type Logger } from "@protean/logger";
import assert from "node:assert";

export interface OrchestratorConfig {
  model: LanguageModel;
  skillsList: Skill<unknown>[];
  instructionsBuilder: (cfg: {
    skillFrontmatters: { id: string; frontmatter: string }[];
  }) => string;
  baseTools?: { [k: string]: Tool };
  agentId?: string;
}

export async function createSkillOrchestrator(
  cfg: OrchestratorConfig,
  logger: Logger,
): Promise<ToolLoopAgent> {
  const skillsRegistry = new Map(
    cfg.skillsList.map((skill) => [skill.id, skill]),
  );
  const activeSkillIds = new Set<string>();
  const allTools: { [k: string]: Tool } = {};
  const alwaysActiveTools: string[] = [];

  if (cfg.baseTools) {
    Object.keys(cfg.baseTools).forEach((toolName) => {
      assert(cfg.baseTools, "cfg.tools should be defined");
      allTools[toolName] = cfg.baseTools[toolName];
      alwaysActiveTools.push(toolName);
    });
  }

  skillsRegistry.forEach((s) => {
    Object.keys(s.tools).forEach((toolName) => {
      if (allTools[toolName]) {
        logger.error("Duplicated tool name in skills");
        // TODO: figure out a solution for this
        throw new Error("Skills have duplicated tool names!");
        // // Creating a namespaced tool name for duplicated tools
        // allTools[s.id + "__" + toolName] = s.tools[toolName];
        // return;
      }
      allTools[toolName] = s.tools[toolName];
    });
  });

  logger.debug(`[Orchestrator] Initialized`, {
    agentId: cfg.agentId ?? "anonymous",
    baseTools: alwaysActiveTools,
    skills: Array.from(skillsRegistry.keys()),
    totalToolCount: Object.keys(allTools).length,
  });

  function loadSkill(id: string): {
    skillId: string;
    instructions: string;
    error: string | null;
  } {
    logger.info(`[Orchestrator] Skill load requested`, { skillId: id });

    const skill = skillsRegistry.get(id);
    if (!skill) {
      logger.warn(`[Orchestrator] Skill not found`, {
        skillId: id,
        available: Array.from(skillsRegistry.keys()),
      });
      return {
        skillId: id,
        instructions: "",
        error: "The skill is not found! Please provide a valid skill id.",
      };
    }

    if (activeSkillIds.has(id)) {
      logger.debug(`[Orchestrator] Skill already loaded`, { skillId: id });
      return {
        skillId: id,
        instructions: `Skill "${id}" is already loaded.`,
        error: null,
      };
    }

    activeSkillIds.add(id);
    const toolNames = Object.keys(skill.tools);
    logger.info(`[Orchestrator] Skill loaded`, {
      skillId: id,
      activatedTools: toolNames,
      activeSkillCount: activeSkillIds.size,
    });
    return {
      skillId: id,
      instructions: skill.instructions,
      error: null,
    };
  }

  const skillTool = tool({
    description:
      "Load a skill to activate its tools and receive usage instructions. Pass the skill ID from the Available Skills list.",
    inputSchema: z.object({
      id: z.string().describe("The skill ID to load (e.g. 'workspace-skill')."),
    }),
    execute: async (args) => loadSkill(args.id),
  });

  allTools["Skill"] = skillTool;
  alwaysActiveTools.push("Skill");

  const agent = new ToolLoopAgent({
    id: cfg.agentId || undefined,
    model: cfg.model,
    instructions: cfg.instructionsBuilder({
      skillFrontmatters: cfg.skillsList.map(({ id, frontmatter }) => ({
        id,
        frontmatter,
      })),
    }),
    toolChoice: "auto",
    tools: allTools,
    activeTools: alwaysActiveTools.slice(),
    stopWhen: stepCountIs(100),
    prepareStep: async () => {
      const activeTools = alwaysActiveTools.slice();

      for (const id of activeSkillIds) {
        const skill = skillsRegistry.get(id);
        if (!skill) {
          continue;
        }
        activeTools.push(...Object.keys(skill.tools));
      }

      return {
        activeTools,
      };
    },
  });

  return agent;
}
