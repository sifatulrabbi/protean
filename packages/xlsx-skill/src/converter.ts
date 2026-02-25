import { type Logger } from "@protean/logger";
import { type FS } from "@protean/vfs";

export interface XlsxConvertResult {
  workbookJsonPath: string;
  sheetsDir: string;
  sheetFiles: string[];
  outputDir: string;
}

export interface XlsxModification {
  sheet: string;
  cell: string;
  value: string | number;
  formula?: string;
}

export interface XlsxConverter {
  toJsonl(xlsxPath: string): Promise<XlsxConvertResult>;
  modify(
    xlsxPath: string,
    modifications: XlsxModification[],
  ): Promise<{ outputPath: string }>;
}

export function createStubXlsxConverter(
  _fsClient: FS,
  _outputDir: string,
  _logger: Logger,
): XlsxConverter {
  return {
    toJsonl: async () => {
      throw new Error("XlsxConverter.toJsonl is not implemented");
    },
    modify: async () => {
      throw new Error("XlsxConverter.modify is not implemented");
    },
  };
}
