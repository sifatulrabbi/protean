import { tool } from "ai";
import { z } from "zod";
import { Skill } from "@protean/skill";
import { type Logger } from "@protean/logger";
import { tryCatch } from "@protean/utils";
import { type FS } from "@protean/vfs";

import { type PptxConverter } from "./converter";
import { pptxSkillDescription, pptxSkillInstructions } from "./instructions";

export interface PptxSkillDeps {
  fsClient: FS;
  converter: PptxConverter;
  logger: Logger;
  outputDir?: string;
}

const DEFAULT_OUTPUT_DIR = "/tmp/converted-pptx-files/";

export class PptxSkill extends Skill<PptxSkillDeps> {
  constructor(dependencies: PptxSkillDeps) {
    super(
      {
        id: "pptx-skill",
        description: pptxSkillDescription,
        instructions: pptxSkillInstructions,
        dependencies,
      },
      dependencies.logger,
    );
  }

  get tools() {
    return {
      PptxToMarkdown: this.PptxToMarkdown,
      PptxToImages: this.PptxToImages,
      ModifyPptxWithJson: this.ModifyPptxWithJson,
    };
  }

  PptxToMarkdown = tool({
    description:
      "Convert a PPTX file to Markdown with slide and element IDs for referencing in modifications.",
    inputSchema: z.object({
      pptxPath: z.string().describe("Path to the PPTX file to convert."),
    }),
    execute: async ({ pptxPath }) => {
      this.logger.debug('Running pptx operation "PptxToMarkdown"', {
        path: pptxPath,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.converter.toMarkdown(pptxPath),
      );
      if (error) {
        this.logger.error('Pptx operation "PptxToMarkdown" failed', {
          path: pptxPath,
          error,
        });
        return {
          error: `Pptx operation "PptxToMarkdown" failed for path "${pptxPath}": ${error.message || "Unknown error"}`,
          pptxPath,
        };
      }

      return { error: null, pptxPath, ...result };
    },
  });

  PptxToImages = tool({
    description:
      "Convert all slides of a PPTX file to PNG images for visual inspection.",
    inputSchema: z.object({
      pptxPath: z.string().describe("Path to the PPTX file to convert."),
    }),
    execute: async ({ pptxPath }) => {
      this.logger.debug('Running pptx operation "PptxToImages"', {
        path: pptxPath,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.converter.toImages(pptxPath),
      );
      if (error) {
        this.logger.error('Pptx operation "PptxToImages" failed', {
          path: pptxPath,
          error,
        });
        return {
          error: `Pptx operation "PptxToImages" failed for path "${pptxPath}": ${error.message || "Unknown error"}`,
          pptxPath,
        };
      }

      return { error: null, pptxPath, ...result };
    },
  });

  ModifyPptxWithJson = tool({
    description:
      "Apply JSON modifications to a PPTX file. Each modification references an element ID and specifies an action.",
    inputSchema: z.object({
      pptxPath: z.string().describe("Path to the PPTX file to modify."),
      modifications: z
        .array(
          z.object({
            elementId: z.string().describe("Element ID from Markdown output."),
            action: z
              .enum(["replace", "delete", "insertAfter", "insertBefore"])
              .describe("The modification action to perform."),
            content: z
              .string()
              .optional()
              .describe(
                "New content for replace/insert actions. Not needed for delete.",
              ),
          }),
        )
        .describe("Array of modifications to apply."),
    }),
    execute: async ({ pptxPath, modifications }) => {
      this.logger.debug('Running pptx operation "ModifyPptxWithJson"', {
        path: pptxPath,
        modificationCount: modifications.length,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.converter.modify(pptxPath, modifications),
      );
      if (error) {
        this.logger.error('Pptx operation "ModifyPptxWithJson" failed', {
          path: pptxPath,
          error,
        });
        return {
          error: `Pptx operation "ModifyPptxWithJson" failed for path "${pptxPath}": ${error.message || "Unknown error"}`,
          pptxPath,
        };
      }

      return { error: null, pptxPath, ...result };
    },
  });
}
