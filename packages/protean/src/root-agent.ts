import { tavilySearch, tavilyExtract } from "@tavily/ai-sdk";
import type { ModelSelection } from "@protean/model-catalog";
import type { Logger } from "@protean/logger";
import type { Skill } from "@protean/skill";
import type { FS } from "@protean/vfs";
import { WorkspaceSkill } from "@protean/workspace-skill";
import { PptxSkill } from "@protean/pptx-skill";
import { XlsxSkill } from "@protean/xlsx-skill";
import { ResearchSkill } from "@protean/research-skill";
import { DocxSkill, createDocxConverter } from "@protean/docx-skill";
import { createStubPptxConverter } from "@protean/pptx-skill";
import { createStubXlsxConverter } from "@protean/xlsx-skill";

import { createModelFromSelection } from "./providers-and-models";
import { createSkillOrchestrator } from "./skill-orchestrator";
import { createSubAgentTools } from "./services/sub-agent";
import { buildRootAgentPrompt } from "./prompts/root-agent-prompt";

const webSearchTools = {
  WebSearchGeneral: tavilySearch({
    searchDepth: "basic",
    includeAnswer: true,
    maxResults: 20,
    topic: "general",
  }),
  WebSearchNews: tavilySearch({
    searchDepth: "basic",
    includeAnswer: true,
    maxResults: 20,
    topic: "news",
  }),
  WebFetchUrlContent: tavilyExtract({
    extractDepth: "basic",
    format: "markdown",
  }),
};

export interface RootAgentOpts {
  fs: FS;
  modelSelection: ModelSelection;
}

function createWorkspaceTools(opts: RootAgentOpts, logger: Logger) {
  const workspaceSkill = new WorkspaceSkill({ fsClient: opts.fs, logger });
  return workspaceSkill.tools;
}

async function createSkills(opts: RootAgentOpts, logger: Logger) {
  const skills: Skill<unknown>[] = [
    new DocxSkill({
      fsClient: opts.fs,
      converter: createDocxConverter(
        opts.fs,
        "/tmp/converted-docx-files/",
        logger,
      ),
      logger,
    }),
    new PptxSkill({
      fsClient: opts.fs,
      converter: createStubPptxConverter(
        opts.fs,
        "/tmp/converted-pptx-files/",
        logger,
      ),
      logger,
    }),
    new XlsxSkill({
      fsClient: opts.fs,
      converter: createStubXlsxConverter(
        opts.fs,
        "/tmp/converted-xlsx-files/",
        logger,
      ),
      logger,
    }),
    new ResearchSkill({ logger }),
  ];

  return skills;
}

export async function createRootAgent(opts: RootAgentOpts, logger: Logger) {
  const { model } = createModelFromSelection(opts.modelSelection, logger);
  const baseTools = {
    ...webSearchTools,
    ...createWorkspaceTools(opts, logger),
  };
  const skillsList = await createSkills(opts, logger);
  const subAgentTools = createSubAgentTools(
    { skillsList, model, baseTools },
    logger,
  );
  const agent = await createSkillOrchestrator(
    {
      agentId: "protean-agent",
      model: model,
      instructionsBuilder: buildRootAgentPrompt,
      skillsList,
      baseTools: { ...baseTools, ...subAgentTools },
    },
    logger,
  );

  return agent;
}
