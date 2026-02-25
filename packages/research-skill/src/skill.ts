import { type Tool } from "ai";
import { Skill } from "@protean/skill";
import { type Logger } from "@protean/logger";

import { description, instructions } from "./instructions";

export interface ResearchSkillDeps {
  logger: Logger;
}

export class ResearchSkill extends Skill<ResearchSkillDeps> {
  constructor(dependencies: ResearchSkillDeps) {
    super(
      {
        id: "research-skill",
        description: description,
        instructions: instructions,
        dependencies: dependencies,
      },
      dependencies.logger,
    );
  }

  get tools(): Record<string, Tool> {
    return {};
  }
}
