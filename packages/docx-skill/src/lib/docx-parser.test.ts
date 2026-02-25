import { describe, it, expect, beforeAll } from "bun:test";
import path from "node:path";
import { createLocalFs } from "@protean/vfs";
import { parseDocx } from "./docx-parser";

const FIXTURES_DIR = path.resolve(import.meta.dirname, "../__fixtures__");

describe("parseDocx", () => {
  let fixtureBuffer: Buffer;

  beforeAll(async () => {
    const fixturesFs = await createLocalFs(FIXTURES_DIR);
    fixtureBuffer = await fixturesFs.readFileBuffer("simple.docx");
  });

  it("parses simple.docx without errors", async () => {
    const result = await parseDocx(fixtureBuffer);

    expect(result.nodes).toBeDefined();
    expect(result.nodes.length).toBeGreaterThan(0);
    expect(result.locationMap.size).toBeGreaterThan(0);
    expect(result.rels.size).toBeGreaterThan(0);
    expect(result.zip).toBeDefined();
  });

  it("assigns correct element IDs to body-level paragraphs", async () => {
    const { nodes } = await parseDocx(fixtureBuffer);

    // First node: Heading1 "Introduction" → p_0
    const p0 = nodes[0];
    expect(p0.type).toBe("paragraph");
    expect(p0.id).toBe("p_0");
    if (p0.type === "paragraph") {
      expect(p0.style).toBe("Heading1");
      expect(p0.runs[0].text).toBe("Introduction");
    }

    // Second node: normal paragraph → p_1
    const p1 = nodes[1];
    expect(p1.type).toBe("paragraph");
    expect(p1.id).toBe("p_1");
  });

  it("parses text runs with formatting", async () => {
    const { nodes } = await parseDocx(fixtureBuffer);

    // p_1: "This is a bold and italic paragraph."
    const p1 = nodes[1];
    expect(p1.type).toBe("paragraph");
    if (p1.type === "paragraph") {
      expect(p1.runs.length).toBe(5);

      expect(p1.runs[0].text).toBe("This is a ");
      expect(p1.runs[0].bold).toBeUndefined();

      expect(p1.runs[1].text).toBe("bold");
      expect(p1.runs[1].bold).toBe(true);

      expect(p1.runs[2].text).toBe(" and ");

      expect(p1.runs[3].text).toBe("italic");
      expect(p1.runs[3].italic).toBe(true);

      expect(p1.runs[4].text).toBe(" paragraph.");
    }
  });

  it("parses heading styles", async () => {
    const { nodes } = await parseDocx(fixtureBuffer);

    // p_0: Heading1
    expect(nodes[0].type).toBe("paragraph");
    if (nodes[0].type === "paragraph") {
      expect(nodes[0].style).toBe("Heading1");
    }

    // p_2: Heading2 "Details"
    expect(nodes[2].type).toBe("paragraph");
    if (nodes[2].type === "paragraph") {
      expect(nodes[2].style).toBe("Heading2");
    }
  });

  it("parses hyperlinks with relationship lookup", async () => {
    const { nodes } = await parseDocx(fixtureBuffer);

    // p_3: "Visit Example Site for more info."
    const p3 = nodes[3];
    expect(p3.type).toBe("paragraph");
    if (p3.type === "paragraph") {
      const hyperlinkRun = p3.runs.find((r) => r.hyperlink);
      expect(hyperlinkRun).toBeDefined();
      expect(hyperlinkRun!.text).toBe("Example Site");
      expect(hyperlinkRun!.hyperlink).toBe("https://example.com");
    }
  });

  it("parses tables with correct IDs", async () => {
    const { nodes } = await parseDocx(fixtureBuffer);

    // Find the table (should be tbl_0)
    const table = nodes.find((n) => n.type === "table");
    expect(table).toBeDefined();
    expect(table!.id).toBe("tbl_0");

    if (table?.type === "table") {
      expect(table.rows.length).toBe(3); // header + 2 data rows

      // Check cell IDs
      expect(table.rows[0].cells[0].id).toBe("tbl_0_r0_c0");
      expect(table.rows[0].cells[1].id).toBe("tbl_0_r0_c1");
      expect(table.rows[1].cells[0].id).toBe("tbl_0_r1_c0");

      // Check cell content
      const headerCell = table.rows[0].cells[0];
      expect(headerCell.children.length).toBe(1);
      const cellP = headerCell.children[0];
      expect(cellP.type).toBe("paragraph");
      if (cellP.type === "paragraph") {
        expect(cellP.runs[0].text).toBe("Name");
        expect(cellP.runs[0].bold).toBe(true);
      }
    }
  });

  it("assigns correct IDs to paragraphs inside table cells", async () => {
    const { nodes, locationMap } = await parseDocx(fixtureBuffer);

    const table = nodes.find((n) => n.type === "table");
    if (table?.type === "table") {
      // Paragraph inside tbl_0_r0_c0 should be tbl_0_r0_c0_p0
      const cellP = table.rows[0].cells[0].children[0];
      expect(cellP.id).toBe("tbl_0_r0_c0_p0");
      expect(locationMap.has("tbl_0_r0_c0_p0")).toBe(true);
    }
  });

  it("parses numbered list paragraphs", async () => {
    const { nodes } = await parseDocx(fixtureBuffer);

    // Find list items (they have numbering property)
    const listItems = nodes.filter(
      (n) => n.type === "paragraph" && n.numbering,
    );
    expect(listItems.length).toBe(3);

    if (listItems[0].type === "paragraph") {
      expect(listItems[0].numbering).toBeDefined();
      expect(listItems[0].numbering!.numId).toBe("2");
      expect(listItems[0].numbering!.level).toBe(0);
      expect(listItems[0].runs[0].text).toBe("First item");
    }
  });

  it("builds a complete locationMap", async () => {
    const { locationMap } = await parseDocx(fixtureBuffer);

    // Should have entries for all body-level elements + cell paragraphs
    expect(locationMap.has("p_0")).toBe(true);
    expect(locationMap.has("p_1")).toBe(true);
    expect(locationMap.has("tbl_0")).toBe(true);
    expect(locationMap.get("p_0")!.tag).toBe("w:p");
    expect(locationMap.get("tbl_0")!.tag).toBe("w:tbl");
  });

  it("parses relationship map", async () => {
    const { rels } = await parseDocx(fixtureBuffer);

    // Should have the hyperlink relationship
    const hyperlinkEntry = [...rels.entries()].find(
      ([_, target]) => target === "https://example.com",
    );
    expect(hyperlinkEntry).toBeDefined();
  });

  it("throws on invalid buffer", async () => {
    await expect(parseDocx(Buffer.from("not a zip"))).rejects.toThrow();
  });
});
