import { type Logger } from "@protean/logger";
import { type FS } from "@protean/vfs";

export interface PptxConvertResult {
  markdownPath: string;
  outputDir: string;
}

export interface PptxImagesToResult {
  imageDir: string;
  slides: string[];
  outputDir: string;
}

export interface PptxModification {
  elementId: string;
  action: "replace" | "delete" | "insertAfter" | "insertBefore";
  content?: string;
}

export interface PptxConverter {
  toMarkdown(pptxPath: string): Promise<PptxConvertResult>;
  toImages(pptxPath: string): Promise<PptxImagesToResult>;
  modify(
    pptxPath: string,
    modifications: PptxModification[],
  ): Promise<{ outputPath: string }>;
}

export function createStubPptxConverter(
  _fsClient: FS,
  _outputDir: string,
  _logger: Logger,
): PptxConverter {
  return {
    toMarkdown: async () => {
      throw new Error("PptxConverter.toMarkdown is not implemented");
    },
    toImages: async () => {
      throw new Error("PptxConverter.toImages is not implemented");
    },
    modify: async () => {
      throw new Error("PptxConverter.modify is not implemented");
    },
  };
}
