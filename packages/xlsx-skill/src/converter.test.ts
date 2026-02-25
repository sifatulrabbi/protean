import { describe, test, expect } from "bun:test";
import { type FS } from "@protean/vfs";
import { createStubXlsxConverter, type XlsxConverter } from "./converter";

const noopLogger = {
  info: () => {},
  debug: () => {},
  warn: () => {},
  error: () => {},
} as any;

const mockFs = {} as FS;

describe("createStubXlsxConverter", () => {
  let converter: XlsxConverter;

  test("returns an object implementing XlsxConverter", () => {
    converter = createStubXlsxConverter(mockFs, "/tmp/test-xlsx", noopLogger);
    expect(converter).toBeDefined();
    expect(typeof converter.toJsonl).toBe("function");
    expect(typeof converter.modify).toBe("function");
  });

  test("toJsonl throws not implemented", async () => {
    converter = createStubXlsxConverter(mockFs, "/tmp/test-xlsx", noopLogger);
    expect(converter.toJsonl("/fake/file.xlsx")).rejects.toThrow(
      /not implemented/,
    );
  });

  test("modify throws not implemented", async () => {
    converter = createStubXlsxConverter(mockFs, "/tmp/test-xlsx", noopLogger);
    expect(
      converter.modify("/fake/file.xlsx", [
        { sheet: "Sheet1", cell: "A1", value: "hello" },
      ]),
    ).rejects.toThrow(/not implemented/);
  });
});
