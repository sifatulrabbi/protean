import path from "node:path";
import { type Logger } from "@protean/logger";
import { type FS } from "@protean/vfs";
import { parseDocx } from "./lib/docx-parser";
import { nodesToMarkdown } from "./lib/docx-to-markdown";
import { applyModifications } from "./lib/docx-modifier";
import { docxToImages } from "./lib/docx-to-images";
import { generateDocxFromCode } from "./lib/docx-from-code";

export interface DocxConvertResult {
  markdownPath: string;
  outputDir: string;
}

export interface DocxImagesToResult {
  imageDir: string;
  pages: string[];
  outputDir: string;
}

export interface DocxModification {
  elementId: string;
  action: "replace" | "delete" | "insertAfter" | "insertBefore";
  content?: string;
}

export interface DocxGenerateResult {
  outputPath: string;
}

export interface DocxConverter {
  toMarkdown(docxPath: string): Promise<DocxConvertResult>;
  toImages(docxPath: string): Promise<DocxImagesToResult>;
  modify(
    docxPath: string,
    modifications: DocxModification[],
  ): Promise<{ outputPath: string }>;
  generateFromCode(
    code: string,
    outputPath: string,
  ): Promise<DocxGenerateResult>;
}

export function createDocxConverter(
  fsClient: FS,
  outputDir: string,
  logger: Logger,
): DocxConverter {
  return {
    async toMarkdown(docxPath: string): Promise<DocxConvertResult> {
      logger.debug("DocxConverter.toMarkdown", { docxPath });

      const buffer = await fsClient.readFileBuffer(docxPath);
      const { nodes, rels } = await parseDocx(buffer);
      const markdown = nodesToMarkdown(nodes, rels);

      const slug = path.basename(docxPath, path.extname(docxPath));
      const outDir = path.join(outputDir, slug);
      await fsClient.mkdir(outDir);

      const mdPath = path.join(outDir, "content.md");
      await fsClient.writeFile(mdPath, markdown);

      logger.debug("DocxConverter.toMarkdown complete", {
        markdownPath: mdPath,
      });
      return { markdownPath: mdPath, outputDir: outDir };
    },

    async toImages(docxPath: string): Promise<DocxImagesToResult> {
      logger.debug("DocxConverter.toImages", { docxPath });

      const slug = path.basename(docxPath, path.extname(docxPath));
      const outDir = path.join(outputDir, slug);
      const imgDir = path.join(outDir, "page-images");

      const { pages } = await docxToImages(docxPath, imgDir, fsClient);

      logger.debug("DocxConverter.toImages complete", {
        pageCount: pages.length,
      });
      return { imageDir: imgDir, pages, outputDir: outDir };
    },

    async modify(
      docxPath: string,
      modifications: DocxModification[],
    ): Promise<{ outputPath: string }> {
      logger.debug("DocxConverter.modify", {
        docxPath,
        modificationCount: modifications.length,
      });

      const buffer = await fsClient.readFileBuffer(docxPath);
      const parsed = await parseDocx(buffer);
      const newBuffer = await applyModifications(parsed, modifications);

      const slug = path.basename(docxPath, path.extname(docxPath));
      const outPath = path.join(
        path.dirname(docxPath),
        `${slug}-modified.docx`,
      );
      await fsClient.writeFileBuffer(outPath, newBuffer);

      logger.debug("DocxConverter.modify complete", { outputPath: outPath });
      return { outputPath: outPath };
    },

    async generateFromCode(
      code: string,
      outputPath: string,
    ): Promise<DocxGenerateResult> {
      logger.debug("DocxConverter.generateFromCode", { outputPath });
      const result = await generateDocxFromCode(code, outputPath, fsClient);
      logger.debug("DocxConverter.generateFromCode complete", {
        outputPath: result.outputPath,
      });
      return result;
    },
  };
}

/** @deprecated Use createDocxConverter instead */
export function createStubDocxConverter(
  _fsClient: FS,
  _outputDir: string,
  _logger: Logger,
): DocxConverter {
  return {
    toMarkdown: async () => {
      throw new Error("DocxConverter.toMarkdown is not implemented");
    },
    toImages: async () => {
      throw new Error("DocxConverter.toImages is not implemented");
    },
    modify: async () => {
      throw new Error("DocxConverter.modify is not implemented");
    },
    generateFromCode: async () => {
      throw new Error("DocxConverter.generateFromCode is not implemented");
    },
  };
}
