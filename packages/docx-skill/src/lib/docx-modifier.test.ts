import { describe, it, expect, beforeAll } from "bun:test";
import path from "node:path";
import { createLocalFs } from "@protean/vfs";
import { parseDocx } from "./docx-parser";
import { applyModifications } from "./docx-modifier";
import type { DocxModification } from "../converter";

const FIXTURES_DIR = path.resolve(import.meta.dirname, "../__fixtures__");

let fixtureBuffer: Buffer;

async function loadSimpleDocx() {
  return parseDocx(fixtureBuffer);
}

describe("applyModifications", () => {
  beforeAll(async () => {
    const fixturesFs = await createLocalFs(FIXTURES_DIR);
    fixtureBuffer = await fixturesFs.readFileBuffer("simple.docx");
  });

  it("replaces text in a paragraph (p_0)", async () => {
    const parsed = await loadSimpleDocx();
    const modifications: DocxModification[] = [
      { elementId: "p_0", action: "replace", content: "New Title" },
    ];

    const newBuffer = await applyModifications(parsed, modifications);
    const reparsed = await parseDocx(newBuffer);

    const p0 = reparsed.nodes[0];
    expect(p0.type).toBe("paragraph");
    if (p0.type === "paragraph") {
      expect(p0.runs.length).toBe(1);
      expect(p0.runs[0].text).toBe("New Title");
    }
  });

  it("replaces text in a paragraph with multiple runs (p_1)", async () => {
    const parsed = await loadSimpleDocx();
    const modifications: DocxModification[] = [
      { elementId: "p_1", action: "replace", content: "Simple text now" },
    ];

    const newBuffer = await applyModifications(parsed, modifications);
    const reparsed = await parseDocx(newBuffer);

    const p1 = reparsed.nodes[1];
    expect(p1.type).toBe("paragraph");
    if (p1.type === "paragraph") {
      expect(p1.runs.length).toBe(1);
      expect(p1.runs[0].text).toBe("Simple text now");
    }
  });

  it("deletes an element and reduces node count", async () => {
    const parsed = await loadSimpleDocx();
    const originalCount = parsed.nodes.length;

    const modifications: DocxModification[] = [
      { elementId: "p_1", action: "delete" },
    ];

    const newBuffer = await applyModifications(parsed, modifications);
    const reparsed = await parseDocx(newBuffer);

    expect(reparsed.nodes.length).toBe(originalCount - 1);
    // p_0 should still be "Introduction"
    const first = reparsed.nodes[0];
    expect(first.type).toBe("paragraph");
    if (first.type === "paragraph") {
      expect(first.runs[0].text).toBe("Introduction");
    }
  });

  it("inserts a paragraph after an element", async () => {
    const parsed = await loadSimpleDocx();
    const originalCount = parsed.nodes.length;

    const modifications: DocxModification[] = [
      { elementId: "p_0", action: "insertAfter", content: "Inserted After" },
    ];

    const newBuffer = await applyModifications(parsed, modifications);
    const reparsed = await parseDocx(newBuffer);

    expect(reparsed.nodes.length).toBe(originalCount + 1);
    // The inserted paragraph should be at index 1
    const inserted = reparsed.nodes[1];
    expect(inserted.type).toBe("paragraph");
    if (inserted.type === "paragraph") {
      expect(inserted.runs[0].text).toBe("Inserted After");
    }
  });

  it("inserts a paragraph before an element", async () => {
    const parsed = await loadSimpleDocx();
    const originalCount = parsed.nodes.length;

    const modifications: DocxModification[] = [
      { elementId: "p_0", action: "insertBefore", content: "Inserted Before" },
    ];

    const newBuffer = await applyModifications(parsed, modifications);
    const reparsed = await parseDocx(newBuffer);

    expect(reparsed.nodes.length).toBe(originalCount + 1);
    // The inserted paragraph should be at index 0
    const inserted = reparsed.nodes[0];
    expect(inserted.type).toBe("paragraph");
    if (inserted.type === "paragraph") {
      expect(inserted.runs[0].text).toBe("Inserted Before");
    }
    // Original p_0 is now at index 1
    const original = reparsed.nodes[1];
    expect(original.type).toBe("paragraph");
    if (original.type === "paragraph") {
      expect(original.runs[0].text).toBe("Introduction");
    }
  });

  it("throws for unknown element ID", async () => {
    const parsed = await loadSimpleDocx();
    const modifications: DocxModification[] = [
      { elementId: "nonexistent_99", action: "delete" },
    ];

    await expect(applyModifications(parsed, modifications)).rejects.toThrow(
      'Element ID "nonexistent_99" not found in locationMap',
    );
  });

  it("round-trip: parse → modify → reparse → verify", async () => {
    const parsed = await loadSimpleDocx();

    // Apply multiple modifications
    const modifications: DocxModification[] = [
      { elementId: "p_0", action: "replace", content: "Modified Heading" },
      {
        elementId: "p_2",
        action: "insertAfter",
        content: "New paragraph after heading 2",
      },
    ];

    const newBuffer = await applyModifications(parsed, modifications);
    const reparsed = await parseDocx(newBuffer);

    // Verify the replace worked
    const p0 = reparsed.nodes[0];
    expect(p0.type).toBe("paragraph");
    if (p0.type === "paragraph") {
      expect(p0.runs[0].text).toBe("Modified Heading");
      // Should still have Heading1 style (formatting preserved)
      expect(p0.style).toBe("Heading1");
    }

    // Verify the insert worked — original p_2 is "Details" heading
    // After insert, the new paragraph should be at index 3
    const inserted = reparsed.nodes[3];
    expect(inserted.type).toBe("paragraph");
    if (inserted.type === "paragraph") {
      expect(inserted.runs[0].text).toBe("New paragraph after heading 2");
    }

    // Verify the table is still intact
    const table = reparsed.nodes.find((n) => n.type === "table");
    expect(table).toBeDefined();
    if (table?.type === "table") {
      expect(table.rows.length).toBe(3);
    }
  });

  it("handles multiple deletions in correct order", async () => {
    const parsed = await loadSimpleDocx();
    const originalCount = parsed.nodes.length;

    // Delete p_0 and p_1 (both body-level, adjacent)
    const modifications: DocxModification[] = [
      { elementId: "p_0", action: "delete" },
      { elementId: "p_1", action: "delete" },
    ];

    const newBuffer = await applyModifications(parsed, modifications);
    const reparsed = await parseDocx(newBuffer);

    expect(reparsed.nodes.length).toBe(originalCount - 2);
    // First node should now be what was originally p_2 (Heading2 "Details")
    const first = reparsed.nodes[0];
    expect(first.type).toBe("paragraph");
    if (first.type === "paragraph") {
      expect(first.style).toBe("Heading2");
    }
  });

  it("modifies text inside a table cell", async () => {
    const parsed = await loadSimpleDocx();

    // tbl_0_r0_c0_p0 should be "Name" (bold header cell)
    const modifications: DocxModification[] = [
      { elementId: "tbl_0_r0_c0_p0", action: "replace", content: "Full Name" },
    ];

    const newBuffer = await applyModifications(parsed, modifications);
    const reparsed = await parseDocx(newBuffer);

    const table = reparsed.nodes.find((n) => n.type === "table");
    expect(table).toBeDefined();
    if (table?.type === "table") {
      const cellP = table.rows[0].cells[0].children[0];
      expect(cellP.type).toBe("paragraph");
      if (cellP.type === "paragraph") {
        expect(cellP.runs[0].text).toBe("Full Name");
      }
    }
  });
});
