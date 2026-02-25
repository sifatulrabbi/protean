import { describe, test, expect } from "bun:test";
import { type FS } from "@protean/vfs";
import { createStubPptxConverter, type PptxConverter } from "./converter";

const noopLogger = {
  info: () => {},
  debug: () => {},
  warn: () => {},
  error: () => {},
} as any;

const mockFs = {} as FS;

describe("createStubPptxConverter", () => {
  let converter: PptxConverter;

  test("returns an object implementing PptxConverter", () => {
    converter = createStubPptxConverter(mockFs, "/tmp/test-pptx", noopLogger);
    expect(converter).toBeDefined();
    expect(typeof converter.toMarkdown).toBe("function");
    expect(typeof converter.toImages).toBe("function");
    expect(typeof converter.modify).toBe("function");
  });

  test("toMarkdown throws not implemented", async () => {
    converter = createStubPptxConverter(mockFs, "/tmp/test-pptx", noopLogger);
    expect(converter.toMarkdown("/fake/file.pptx")).rejects.toThrow(
      /not implemented/,
    );
  });

  test("toImages throws not implemented", async () => {
    converter = createStubPptxConverter(mockFs, "/tmp/test-pptx", noopLogger);
    expect(converter.toImages("/fake/file.pptx")).rejects.toThrow(
      /not implemented/,
    );
  });

  test("modify throws not implemented", async () => {
    converter = createStubPptxConverter(mockFs, "/tmp/test-pptx", noopLogger);
    expect(
      converter.modify("/fake/file.pptx", [
        { elementId: "slide_1_text_1", action: "replace", content: "new text" },
      ]),
    ).rejects.toThrow(/not implemented/);
  });
});
