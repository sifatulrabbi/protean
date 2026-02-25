import { type Tool } from "ai";
import { type Logger } from "@protean/logger";

export interface SkillParams<D> {
  id: string;
  description: string;
  instructions: string;
  dependencies: D;
  version?: string;
}

export abstract class Skill<D> {
  id: string;
  description: string;
  instructions: string;
  dependencies: D;
  version?: string;
  logger: Logger;

  constructor(
    { id, description, instructions, dependencies, version }: SkillParams<D>,
    logger: Logger,
  ) {
    this.id = id;
    this.description = description;
    this.instructions = instructions;
    this.dependencies = dependencies;
    this.version = version || "1.0.0";
    this.logger = logger;
  }

  abstract get tools(): { [k: string]: Tool };

  get frontmatter() {
    return `---
skill_id: ${this.id}
version: ${this.version}
description: ${this.description}
tools: [${Object.keys(this.tools).join(", ")}]
dependencies: []
---`;
  }
}
