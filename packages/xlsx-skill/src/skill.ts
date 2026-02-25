import { tool } from "ai";
import { z } from "zod";
import { Skill } from "@protean/skill";
import { type Logger } from "@protean/logger";
import { tryCatch } from "@protean/utils";
import { type FS } from "@protean/vfs";

import { type XlsxConverter } from "./converter";
import { xlsxSkillDescription, xlsxSkillInstructions } from "./instructions";

export interface XlsxSkillDeps {
  fsClient: FS;
  converter: XlsxConverter;
  logger: Logger;
  outputDir?: string;
}

const DEFAULT_OUTPUT_DIR = "/tmp/converted-xlsx-files/";

export class XlsxSkill extends Skill<XlsxSkillDeps> {
  constructor(dependencies: XlsxSkillDeps) {
    super(
      {
        id: "xlsx-skill",
        description: xlsxSkillDescription,
        instructions: xlsxSkillInstructions,
        dependencies,
      },
      dependencies.logger,
    );
  }

  get tools() {
    return {
      XlsxToJsonl: this.XlsxToJsonl,
      ModifyXlsxWithJsonl: this.ModifyXlsxWithJsonl,
    };
  }

  XlsxToJsonl = tool({
    description:
      "Convert an XLSX file to JSONL format preserving formulas. Each sheet becomes a separate .jsonl file.",
    inputSchema: z.object({
      xlsxPath: z.string().describe("Path to the XLSX file to convert."),
    }),
    execute: async ({ xlsxPath }) => {
      this.logger.debug('Running xlsx operation "XlsxToJsonl"', {
        path: xlsxPath,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.converter.toJsonl(xlsxPath),
      );
      if (error) {
        this.logger.error('Xlsx operation "XlsxToJsonl" failed', {
          path: xlsxPath,
          error,
        });
        return {
          error: `Xlsx operation "XlsxToJsonl" failed for path "${xlsxPath}": ${error.message || "Unknown error"}`,
          xlsxPath,
        };
      }

      return { error: null, xlsxPath, ...result };
    },
  });

  ModifyXlsxWithJsonl = tool({
    description:
      "Apply JSONL modifications to an XLSX file. Each modification specifies a sheet, cell, value, and optional formula.",
    inputSchema: z.object({
      xlsxPath: z.string().describe("Path to the XLSX file to modify."),
      modifications: z
        .array(
          z.object({
            sheet: z.string().describe("Sheet name to modify."),
            cell: z
              .string()
              .describe('Cell reference in Excel notation (e.g., "A1").'),
            value: z
              .union([z.string(), z.number()])
              .describe("New cell value."),
            formula: z
              .string()
              .optional()
              .describe('Optional formula (e.g., "=SUM(A1:A10)").'),
          }),
        )
        .describe("Array of cell modifications to apply."),
    }),
    execute: async ({ xlsxPath, modifications }) => {
      this.logger.debug('Running xlsx operation "ModifyXlsxWithJsonl"', {
        path: xlsxPath,
        modificationCount: modifications.length,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.converter.modify(xlsxPath, modifications),
      );
      if (error) {
        this.logger.error('Xlsx operation "ModifyXlsxWithJsonl" failed', {
          path: xlsxPath,
          error,
        });
        return {
          error: `Xlsx operation "ModifyXlsxWithJsonl" failed for path "${xlsxPath}": ${error.message || "Unknown error"}`,
          xlsxPath,
        };
      }

      return { error: null, xlsxPath, ...result };
    },
  });
}
