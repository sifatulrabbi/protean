import { describe, test, expect } from "bun:test";
import { type FS } from "@protean/vfs";
import { createStubDocxConverter, type DocxConverter } from "./converter";

const noopLogger = {
  info: () => {},
  debug: () => {},
  warn: () => {},
  error: () => {},
} as any;

const mockFs = {} as FS;

describe("createStubDocxConverter", () => {
  let converter: DocxConverter;

  test("returns an object implementing DocxConverter", () => {
    converter = createStubDocxConverter(mockFs, "/tmp/test-docx", noopLogger);
    expect(converter).toBeDefined();
    expect(typeof converter.toMarkdown).toBe("function");
    expect(typeof converter.toImages).toBe("function");
    expect(typeof converter.modify).toBe("function");
  });

  test("toMarkdown throws not implemented", async () => {
    converter = createStubDocxConverter(mockFs, "/tmp/test-docx", noopLogger);
    expect(converter.toMarkdown("/fake/file.docx")).rejects.toThrow(
      /not implemented/,
    );
  });

  test("toImages throws not implemented", async () => {
    converter = createStubDocxConverter(mockFs, "/tmp/test-docx", noopLogger);
    expect(converter.toImages("/fake/file.docx")).rejects.toThrow(
      /not implemented/,
    );
  });

  test("modify throws not implemented", async () => {
    converter = createStubDocxConverter(mockFs, "/tmp/test-docx", noopLogger);
    expect(
      converter.modify("/fake/file.docx", [
        { elementId: "p_1", action: "replace", content: "new text" },
      ]),
    ).rejects.toThrow(/not implemented/);
  });
});
