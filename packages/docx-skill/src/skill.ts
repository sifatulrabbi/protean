import { tool } from "ai";
import { z } from "zod";
import { Skill } from "@protean/skill";
import { type Logger } from "@protean/logger";
import { tryCatch } from "@protean/utils";
import { type FS } from "@protean/vfs";

import { type DocxConverter } from "./converter";
import { docxSkillDescription, docxSkillInstructions } from "./instructions";

export interface DocxSkillDeps {
  fsClient: FS;
  converter: DocxConverter;
  logger: Logger;
  outputDir?: string;
}

const DEFAULT_OUTPUT_DIR = "/tmp/converted-docx-files/";

export class DocxSkill extends Skill<DocxSkillDeps> {
  constructor(dependencies: DocxSkillDeps) {
    super(
      {
        id: "docx-skill",
        description: docxSkillDescription,
        instructions: docxSkillInstructions,
        dependencies,
      },
      dependencies.logger,
    );

    if (!this.dependencies.outputDir) {
      this.dependencies.outputDir = DEFAULT_OUTPUT_DIR;
    }
  }

  get tools() {
    return {
      DocxToMarkdown: this.DocxToMarkdown,
      DocxToImages: this.DocxToImages,
      ModifyDocxWithJson: this.ModifyDocxWithJson,
      GenerateDocxFromCode: this.GenerateDocxFromCode,
    };
  }

  DocxToMarkdown = tool({
    description:
      "Convert a DOCX file to structured Markdown. Parses headings, bold/italic/underline/strikethrough, hyperlinks, tables, and numbered/bullet lists. Each paragraph and table is tagged with a deterministic element ID comment (e.g. <!-- p_0 -->, <!-- tbl_0 -->) that can be used to target modifications with ModifyDocxWithJson. Returns the path to the generated .md file.",
    inputSchema: z.object({
      docxPath: z
        .string()
        .describe(
          "Workspace-relative path to the .docx file (e.g. 'documents/report.docx').",
        ),
    }),
    execute: async ({ docxPath }) => {
      this.logger.debug('Running docx operation "DocxToMarkdown"', {
        path: docxPath,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.converter.toMarkdown(docxPath),
      );
      if (error) {
        this.logger.error('Docx operation "DocxToMarkdown" failed', {
          path: docxPath,
          error,
        });
        return {
          error: `Docx operation "DocxToMarkdown" failed for path "${docxPath}": ${error.message || "Unknown error"}`,
          docxPath,
        };
      }

      return { error: null, docxPath, ...result };
    },
  });

  DocxToImages = tool({
    description:
      "Render every page of a DOCX file as a high-resolution PNG image (via LibreOffice headless). Useful for verifying visual layout, table formatting, and content that Markdown cannot fully represent. Images are named page-1.png, page-2.png, etc. Requires LibreOffice to be installed.",
    inputSchema: z.object({
      docxPath: z
        .string()
        .describe(
          "Workspace-relative path to the .docx file (e.g. 'documents/report.docx').",
        ),
    }),
    execute: async ({ docxPath }) => {
      this.logger.debug('Running docx operation "DocxToImages"', {
        path: docxPath,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.converter.toImages(docxPath),
      );
      if (error) {
        this.logger.error('Docx operation "DocxToImages" failed', {
          path: docxPath,
          error,
        });
        return {
          error: `Docx operation "DocxToImages" failed for path "${docxPath}": ${error.message || "Unknown error"}`,
          docxPath,
        };
      }

      return { error: null, docxPath, ...result };
    },
  });

  GenerateDocxFromCode = tool({
    description:
      "Generate a new DOCX file from docxjs code. Write TypeScript code that defines a `doc` variable (a docx.Document instance). The docx package is pre-imported — use Document, Paragraph, TextRun, HeadingLevel, Table, TableRow, TableCell, and other docx exports directly. The tool executes the code, packs the document, and writes the .docx file to the specified output path.",
    inputSchema: z.object({
      code: z
        .string()
        .describe(
          "TypeScript code using the docx library that defines a `doc` variable (a docx.Document instance). All docx exports are available as globals.",
        ),
      outputPath: z
        .string()
        .describe(
          "Workspace-relative path for the output .docx file (e.g. 'reports/quarterly.docx').",
        ),
    }),
    execute: async ({ code, outputPath }) => {
      this.logger.debug('Running docx operation "GenerateDocxFromCode"', {
        outputPath,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.converter.generateFromCode(code, outputPath),
      );
      if (error) {
        this.logger.error('Docx operation "GenerateDocxFromCode" failed', {
          outputPath,
          error,
        });
        return {
          error: `Docx operation "GenerateDocxFromCode" failed for path "${outputPath}": ${error.message || "Unknown error"}`,
          outputPath,
        };
      }

      return { error: null, ...result };
    },
  });

  ModifyDocxWithJson = tool({
    description:
      "Apply targeted modifications to a DOCX file using element IDs from DocxToMarkdown output. Supports replacing paragraph text (preserving original formatting like bold and heading style), deleting elements, and inserting new paragraphs before or after existing ones. Multiple modifications can be batched in a single call. Produces a new *-modified.docx file.",
    inputSchema: z.object({
      docxPath: z
        .string()
        .describe(
          "Workspace-relative path to the .docx file to modify (e.g. 'documents/report.docx').",
        ),
      modifications: z
        .array(
          z.object({
            elementId: z
              .string()
              .describe(
                "Element ID from DocxToMarkdown output (e.g. 'p_0', 'tbl_0_r0_c0_p0'). See the Markdown HTML comments to find IDs.",
              ),
            action: z
              .enum(["replace", "delete", "insertAfter", "insertBefore"])
              .describe(
                "Action to perform. 'replace' swaps the element's text content while preserving formatting. 'delete' removes the element entirely. 'insertAfter'/'insertBefore' adds a new plain paragraph adjacent to the target.",
              ),
            content: z
              .string()
              .optional()
              .describe(
                "The new plain text content. Required for replace, insertAfter, and insertBefore. Omit for delete.",
              ),
          }),
        )
        .describe(
          "Array of modifications to apply. Multiple modifications are processed safely regardless of order.",
        ),
    }),
    execute: async ({ docxPath, modifications }) => {
      this.logger.debug('Running docx operation "ModifyDocxWithJson"', {
        path: docxPath,
        modificationCount: modifications.length,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.converter.modify(docxPath, modifications),
      );
      if (error) {
        this.logger.error('Docx operation "ModifyDocxWithJson" failed', {
          path: docxPath,
          error,
        });
        return {
          error: `Docx operation "ModifyDocxWithJson" failed for path "${docxPath}": ${error.message || "Unknown error"}`,
          docxPath,
        };
      }

      return { error: null, docxPath, ...result };
    },
  });
}
