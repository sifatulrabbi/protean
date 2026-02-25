import { describe, expect, test, beforeAll } from "bun:test";
import path from "node:path";
import { createLocalFs } from "@protean/vfs";
import { parseDocx } from "./docx-parser";
import { nodesToMarkdown } from "./docx-to-markdown";
import type { DocxNode, DocxParagraph, DocxTable, DocxRun } from "./types";

const FIXTURES_DIR = path.resolve(import.meta.dirname, "../__fixtures__");

// ─── Fixture-based tests ─────────────────────────────────────────────

describe("nodesToMarkdown (fixture)", () => {
  let markdown: string;
  let nodes: DocxNode[];
  let rels: Map<string, string>;

  beforeAll(async () => {
    const fixturesFs = await createLocalFs(FIXTURES_DIR);
    const buffer = await fixturesFs.readFileBuffer("simple.docx");
    const parsed = await parseDocx(buffer);
    nodes = parsed.nodes;
    rels = parsed.rels;
    markdown = nodesToMarkdown(nodes, rels);
  });

  test("converts headings", () => {
    expect(markdown).toContain("# ");
    // Check that heading content is present
    const lines = markdown.split("\n");
    const h1Lines = lines.filter((l) => l.startsWith("# "));
    expect(h1Lines.length).toBeGreaterThan(0);
  });

  test("converts bold formatting", () => {
    expect(markdown).toMatch(/\*\*[^*]+\*\*/);
  });

  test("converts italic formatting", () => {
    // Match single asterisks that are NOT part of bold (**) or bold-italic (***)
    expect(markdown).toMatch(/(?<!\*)\*[^*]+\*(?!\*)/);
  });

  test("converts hyperlinks", () => {
    expect(markdown).toMatch(/\[.+?\]\(https?:\/\/.+?\)/);
  });

  test("converts tables with pipe syntax", () => {
    expect(markdown).toContain("|");
    // Should have header separator row
    expect(markdown).toMatch(/\|[\s-]+\|/);
  });

  test("converts numbered lists", () => {
    expect(markdown).toMatch(/^\d+\.\s/m);
  });

  test("includes element ID comments", () => {
    expect(markdown).toMatch(/<!-- p_\d+ -->/);
  });

  test("includes table element ID comments", () => {
    expect(markdown).toMatch(/<!-- tbl_\d+ -->/);
  });
});

// ─── Unit tests with synthetic nodes ─────────────────────────────────

describe("nodesToMarkdown (unit)", () => {
  const emptyRels = new Map<string, string>();

  test("heading levels 1-3", () => {
    const nodes: DocxNode[] = [
      makeParagraph("p_0", [{ text: "Title" }], "Heading1"),
      makeParagraph("p_1", [{ text: "Subtitle" }], "Heading2"),
      makeParagraph("p_2", [{ text: "Section" }], "Heading3"),
    ];
    const md = nodesToMarkdown(nodes, emptyRels);
    expect(md).toContain("# Title");
    expect(md).toContain("## Subtitle");
    expect(md).toContain("### Section");
  });

  test("bold and italic", () => {
    const nodes: DocxNode[] = [
      makeParagraph("p_0", [
        { text: "normal " },
        { text: "bold", bold: true },
        { text: " " },
        { text: "italic", italic: true },
        { text: " " },
        { text: "both", bold: true, italic: true },
      ]),
    ];
    const md = nodesToMarkdown(nodes, emptyRels);
    expect(md).toContain("**bold**");
    expect(md).toContain("*italic*");
    expect(md).toContain("***both***");
  });

  test("strikethrough", () => {
    const nodes: DocxNode[] = [
      makeParagraph("p_0", [{ text: "deleted", strike: true }]),
    ];
    const md = nodesToMarkdown(nodes, emptyRels);
    expect(md).toContain("~~deleted~~");
  });

  test("underline", () => {
    const nodes: DocxNode[] = [
      makeParagraph("p_0", [{ text: "underlined", underline: true }]),
    ];
    const md = nodesToMarkdown(nodes, emptyRels);
    expect(md).toContain("<u>underlined</u>");
  });

  test("hyperlinks", () => {
    const nodes: DocxNode[] = [
      makeParagraph("p_0", [
        { text: "Click ", hyperlink: undefined },
        { text: "here", hyperlink: "https://example.com" },
      ]),
    ];
    const md = nodesToMarkdown(nodes, emptyRels);
    expect(md).toContain("[here](https://example.com)");
  });

  test("numbered list", () => {
    const nodes: DocxNode[] = [
      makeListParagraph("p_0", [{ text: "First" }], "decimal", 0),
      makeListParagraph("p_1", [{ text: "Second" }], "decimal", 0),
    ];
    const md = nodesToMarkdown(nodes, emptyRels);
    expect(md).toContain("1. First");
    expect(md).toContain("1. Second");
  });

  test("bullet list", () => {
    const nodes: DocxNode[] = [
      makeListParagraph("p_0", [{ text: "Item A" }], "bullet", 0),
      makeListParagraph("p_1", [{ text: "Item B" }], "bullet", 0),
    ];
    const md = nodesToMarkdown(nodes, emptyRels);
    expect(md).toContain("- Item A");
    expect(md).toContain("- Item B");
  });

  test("nested list indentation", () => {
    const nodes: DocxNode[] = [
      makeListParagraph("p_0", [{ text: "Top" }], "decimal", 0),
      makeListParagraph("p_1", [{ text: "Nested" }], "decimal", 1),
    ];
    const md = nodesToMarkdown(nodes, emptyRels);
    expect(md).toContain("1. Top");
    expect(md).toContain("  1. Nested");
  });

  test("simple table", () => {
    const table: DocxTable = {
      type: "table",
      id: "tbl_0",
      rows: [
        {
          cells: [
            makeCell("tbl_0_r0_c0", "Header 1"),
            makeCell("tbl_0_r0_c1", "Header 2"),
          ],
        },
        {
          cells: [
            makeCell("tbl_0_r1_c0", "Data 1"),
            makeCell("tbl_0_r1_c1", "Data 2"),
          ],
        },
      ],
    };
    const md = nodesToMarkdown([table], emptyRels);
    expect(md).toContain("| Header 1 | Header 2 |");
    expect(md).toContain("| --- | --- |");
    expect(md).toContain("| Data 1 | Data 2 |");
  });

  test("skips empty paragraphs", () => {
    const nodes: DocxNode[] = [
      makeParagraph("p_0", [{ text: "" }]),
      makeParagraph("p_1", [{ text: "content" }]),
    ];
    const md = nodesToMarkdown(nodes, emptyRels);
    expect(md).not.toContain("<!-- p_0 -->");
    expect(md).toContain("<!-- p_1 -->");
  });

  test("element ID comments precede content", () => {
    const nodes: DocxNode[] = [makeParagraph("p_0", [{ text: "Hello" }])];
    const md = nodesToMarkdown(nodes, emptyRels);
    const lines = md.split("\n");
    const commentIdx = lines.findIndex((l) => l === "<!-- p_0 -->");
    expect(commentIdx).toBeGreaterThanOrEqual(0);
    expect(lines[commentIdx + 1]).toBe("Hello");
  });
});

// ─── Helpers ─────────────────────────────────────────────────────────

function makeParagraph(
  id: string,
  runs: DocxRun[],
  style?: string,
): DocxParagraph {
  return { type: "paragraph", id, runs, style };
}

function makeListParagraph(
  id: string,
  runs: DocxRun[],
  format: string,
  level: number,
): DocxParagraph {
  return {
    type: "paragraph",
    id,
    runs,
    style: "ListParagraph",
    numbering: { numId: "1", level, format },
  };
}

function makeCell(
  id: string,
  text: string,
): { id: string; children: DocxNode[] } {
  return {
    id,
    children: [
      {
        type: "paragraph" as const,
        id: `${id}_p0`,
        runs: [{ text }],
      },
    ],
  };
}
