import { describe, it, expect, beforeAll, afterAll } from "bun:test";
import path from "node:path";
import { createDocxConverter, type DocxConverter } from "./converter";
import { createLocalFs, type FS } from "@protean/vfs";
import { noopLogger } from "@protean/logger";
import { parseDocx } from "./lib/docx-parser";
import { nodesToMarkdown } from "./lib/docx-to-markdown";

const FIXTURES_DIR = path.resolve(import.meta.dirname, "./__fixtures__");
const TMP_BASE = path.resolve(import.meta.dirname, "../tmp");

describe("createDocxConverter integration", () => {
  let converter: DocxConverter;
  let workspaceFs: FS;
  let workspaceName: string;
  let tmpFs: FS;

  beforeAll(async () => {
    tmpFs = await createLocalFs(TMP_BASE);
    workspaceName = `docx-test-${Date.now()}`;
    workspaceFs = await createLocalFs(path.join(TMP_BASE, workspaceName));

    // Seed workspace with fixture via VFS
    const fixturesFs = await createLocalFs(FIXTURES_DIR);
    const fixtureBuffer = await fixturesFs.readFileBuffer("simple.docx");
    await workspaceFs.writeFileBuffer("simple.docx", fixtureBuffer);

    converter = createDocxConverter(workspaceFs, "converted", noopLogger);
  });

  afterAll(async () => {
    if (tmpFs && workspaceName) {
      await tmpFs.remove(workspaceName);
    }
  });

  describe("toMarkdown", () => {
    it("converts simple.docx to markdown file", async () => {
      const result = await converter.toMarkdown("simple.docx");

      expect(result.markdownPath).toContain("content.md");
      expect(result.outputDir).toContain("simple");

      const md = await workspaceFs.readFile(result.markdownPath);
      expect(md).toContain("# Introduction");
      expect(md).toContain("**bold**");
      expect(md).toContain("*italic*");
      expect(md).toContain("<!-- p_0 -->");
      expect(md).toContain("<!-- tbl_0 -->");
    });

    it("markdown contains hyperlinks", async () => {
      const result = await converter.toMarkdown("simple.docx");
      const md = await workspaceFs.readFile(result.markdownPath);

      expect(md).toContain("[Example Site](https://example.com)");
    });

    it("markdown contains table", async () => {
      const result = await converter.toMarkdown("simple.docx");
      const md = await workspaceFs.readFile(result.markdownPath);

      expect(md).toContain("| **Name** | **Age** |");
      expect(md).toContain("| Alice | 30 |");
    });

    it("markdown contains numbered list", async () => {
      const result = await converter.toMarkdown("simple.docx");
      const md = await workspaceFs.readFile(result.markdownPath);

      expect(md).toContain("1. First item");
      expect(md).toContain("1. Second item");
    });
  });

  describe("modify", () => {
    it("replaces text in a paragraph", async () => {
      const result = await converter.modify("simple.docx", [
        { elementId: "p_0", action: "replace", content: "New Title" },
      ]);

      expect(result.outputPath).toContain("simple-modified.docx");

      // Re-parse and verify
      const newBuffer = await workspaceFs.readFileBuffer(result.outputPath);
      const { nodes } = await parseDocx(newBuffer);
      const p0 = nodes[0];
      expect(p0.type).toBe("paragraph");
      if (p0.type === "paragraph") {
        expect(p0.runs[0].text).toBe("New Title");
      }
    });

    it("deletes an element", async () => {
      // First count original nodes
      const origBuffer = await workspaceFs.readFileBuffer("simple.docx");
      const { nodes: origNodes } = await parseDocx(origBuffer);
      const origCount = origNodes.length;

      const result = await converter.modify("simple.docx", [
        { elementId: "p_1", action: "delete" },
      ]);

      const newBuffer = await workspaceFs.readFileBuffer(result.outputPath);
      const { nodes: newNodes } = await parseDocx(newBuffer);
      expect(newNodes.length).toBe(origCount - 1);
    });

    it("inserts a paragraph after an element", async () => {
      const result = await converter.modify("simple.docx", [
        {
          elementId: "p_0",
          action: "insertAfter",
          content: "Inserted paragraph",
        },
      ]);

      const newBuffer = await workspaceFs.readFileBuffer(result.outputPath);
      const { nodes } = await parseDocx(newBuffer);

      // p_0 is still the heading, p_1 should now be the inserted paragraph
      expect(nodes[1].type).toBe("paragraph");
      if (nodes[1].type === "paragraph") {
        expect(nodes[1].runs[0].text).toBe("Inserted paragraph");
      }
    });

    it("round-trip: modify and re-convert to markdown", async () => {
      // Modify
      const modResult = await converter.modify("simple.docx", [
        { elementId: "p_0", action: "replace", content: "Updated Heading" },
        {
          elementId: "p_4",
          action: "insertAfter",
          content: "A brand new paragraph",
        },
      ]);

      // Re-parse and convert to markdown
      const newBuffer = await workspaceFs.readFileBuffer(modResult.outputPath);
      const { nodes, rels } = await parseDocx(newBuffer);
      const md = nodesToMarkdown(nodes, rels);

      expect(md).toContain("# Updated Heading");
      expect(md).toContain("A brand new paragraph");
      // Original table should still be intact
      expect(md).toContain("| Alice | 30 |");
    });
  });
});
