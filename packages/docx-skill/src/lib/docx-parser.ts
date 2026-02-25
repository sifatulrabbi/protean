/**
 * Low-level DOCX XML parser.
 *
 * Opens a DOCX file (ZIP archive), parses word/document.xml,
 * walks the XML tree assigning deterministic element IDs,
 * and produces a typed DocxNode[] tree.
 *
 * The same ID assignment logic is used by both toMarkdown and modify,
 * guaranteeing that element IDs are stable across operations.
 */
import JSZip from "jszip";
import { XMLParser, XMLBuilder } from "fast-xml-parser";
import type {
  DocxNode,
  DocxParagraph,
  DocxTable,
  DocxTableCell,
  DocxTableRow,
  DocxRun,
  ParsedDocx,
  XmlElementLocation,
  NumberingInfo,
} from "./types";

// ─── XML Parser Config ───────────────────────────────────────────────
//
// In preserveOrder mode, each XML element is represented as:
//   { "tagName": [ ...children ], ":@": { "@_attr": "value" } }
//
// Children array items follow the same pattern. Text nodes are:
//   { "#text": "some text" }

const parserOptions = {
  ignoreAttributes: false,
  attributeNamePrefix: "@_",
  preserveOrder: true,
  trimValues: false,
  textNodeName: "#text",
  isArray: () => true,
};

export const xmlParser = new XMLParser(parserOptions);

export const xmlBuilder = new XMLBuilder({
  ignoreAttributes: false,
  attributeNamePrefix: "@_",
  preserveOrder: true,
  textNodeName: "#text",
});

// ─── PreserveOrder Helpers ───────────────────────────────────────────
//
// An "element object" in preserveOrder looks like:
//   { "w:p": [ ...children ], ":@": { "@_w:val": "Heading1" } }
//
// - The tag is the first key that isn't ":@" or "#text"
// - Children are the array value of that key
// - Attributes are in the ":@" key

type XmlObj = Record<string, unknown>;

/** Get the tag name of a preserveOrder element object. */
function tagOf(obj: XmlObj): string | undefined {
  for (const key of Object.keys(obj)) {
    if (key !== ":@" && key !== "#text") return key;
  }
  return undefined;
}

/** Get the children array of a preserveOrder element object by tag. */
function childrenOf(obj: XmlObj, tag: string): XmlObj[] {
  const val = obj[tag];
  if (Array.isArray(val)) return val as XmlObj[];
  return [];
}

/** Get the `:@` attributes of a preserveOrder element object. */
function attrsOf(obj: XmlObj): Record<string, string> | undefined {
  const attrs = obj[":@"];
  if (attrs && typeof attrs === "object")
    return attrs as Record<string, string>;
  return undefined;
}

/**
 * Get a single attribute value from an element object.
 * The element's attributes are on its own `:@` key.
 */
function attrVal(obj: XmlObj, attrName: string): string | undefined {
  return attrsOf(obj)?.[attrName];
}

/**
 * Find a child element with a given tag within a children array.
 * Returns the element object (the one containing { tag: [...], ":@": {...} }).
 */
function findChild(children: XmlObj[], tag: string): XmlObj | undefined {
  return children.find((c) => tag in c);
}

/**
 * Find all child elements with a given tag within a children array.
 */
function findChildren(children: XmlObj[], tag: string): XmlObj[] {
  return children.filter((c) => tag in c);
}

// ─── Main Entry ──────────────────────────────────────────────────────

export async function parseDocx(
  buffer: Buffer | ArrayBuffer | Uint8Array,
): Promise<ParsedDocx> {
  const zip = await JSZip.loadAsync(buffer);

  // Parse document.xml
  const docXmlStr = await zip.file("word/document.xml")?.async("string");
  if (!docXmlStr) {
    throw new Error("DOCX file does not contain word/document.xml");
  }

  const parsedXml = xmlParser.parse(docXmlStr) as XmlObj[];

  // Parse relationships
  const rels = await parseRelationships(zip);

  // Parse numbering definitions
  const numberingMap = await parseNumbering(zip);

  // Find w:body
  const docObj = parsedXml.find((item) => "w:document" in item);
  if (!docObj) {
    throw new Error("Could not find <w:document> in document.xml");
  }
  const docChildren = childrenOf(docObj, "w:document");
  const bodyObj = findChild(docChildren, "w:body");
  if (!bodyObj) {
    throw new Error("Could not find <w:body> in document.xml");
  }
  const bodyChildren = childrenOf(bodyObj, "w:body");

  const locationMap = new Map<string, XmlElementLocation>();
  const nodes = walkBodyChildren(
    bodyChildren,
    "",
    locationMap,
    rels,
    numberingMap,
  );

  return { nodes, locationMap, rels, zip, parsedXml, numberingMap };
}

// ─── Relationship Parsing ────────────────────────────────────────────

async function parseRelationships(zip: JSZip): Promise<Map<string, string>> {
  const rels = new Map<string, string>();
  const relsXml = await zip
    .file("word/_rels/document.xml.rels")
    ?.async("string");
  if (!relsXml) return rels;

  const parsed = xmlParser.parse(relsXml) as XmlObj[];

  // Find <Relationships> (may be prefixed with ?xml first)
  const relsObj = parsed.find((item) => "Relationships" in item);
  if (!relsObj) return rels;

  const relChildren = childrenOf(relsObj, "Relationships");
  for (const child of relChildren) {
    if (!("Relationship" in child)) continue;
    const attrs = attrsOf(child);
    const id = attrs?.["@_Id"];
    const target = attrs?.["@_Target"];
    if (id && target) {
      rels.set(id, target);
    }
  }

  return rels;
}

// ─── Numbering Parsing ──────────────────────────────────────────────

async function parseNumbering(zip: JSZip): Promise<Map<string, NumberingInfo>> {
  const map = new Map<string, NumberingInfo>();
  const numXml = await zip.file("word/numbering.xml")?.async("string");
  if (!numXml) return map;

  const parsed = xmlParser.parse(numXml) as XmlObj[];
  const numObj = parsed.find((item) => "w:numbering" in item);
  if (!numObj) return map;
  const numChildren = childrenOf(numObj, "w:numbering");

  // First pass: collect abstractNum definitions
  const abstractNums = new Map<string, NumberingInfo>();
  for (const child of numChildren) {
    if (!("w:abstractNum" in child)) continue;
    const abstractNumId = attrVal(child, "@_w:abstractNumId");
    if (!abstractNumId) continue;

    const levels = new Map<number, { format: string; text: string }>();
    const lvlElements = findChildren(
      childrenOf(child, "w:abstractNum"),
      "w:lvl",
    );
    for (const lvlObj of lvlElements) {
      const ilvl = parseInt(attrVal(lvlObj, "@_w:ilvl") ?? "0", 10);
      const lvlChildren = childrenOf(lvlObj, "w:lvl");

      let format = "bullet";
      let text = "";
      const numFmtObj = findChild(lvlChildren, "w:numFmt");
      if (numFmtObj) format = attrVal(numFmtObj, "@_w:val") ?? "bullet";
      const lvlTextObj = findChild(lvlChildren, "w:lvlText");
      if (lvlTextObj) text = attrVal(lvlTextObj, "@_w:val") ?? "";

      levels.set(ilvl, { format, text });
    }
    abstractNums.set(abstractNumId, { levels });
  }

  // Second pass: map numId → abstractNumId
  for (const child of numChildren) {
    if (!("w:num" in child)) continue;
    const numId = attrVal(child, "@_w:numId");
    if (!numId) continue;

    const numInnerChildren = childrenOf(child, "w:num");
    const abstractNumIdObj = findChild(numInnerChildren, "w:abstractNumId");
    if (!abstractNumIdObj) continue;
    const abstractNumId = attrVal(abstractNumIdObj, "@_w:val");
    if (abstractNumId && abstractNums.has(abstractNumId)) {
      map.set(numId, abstractNums.get(abstractNumId)!);
    }
  }

  return map;
}

// ─── Body Walking ────────────────────────────────────────────────────

function walkBodyChildren(
  bodyChildren: XmlObj[],
  idPrefix: string,
  locationMap: Map<string, XmlElementLocation>,
  rels: Map<string, string>,
  numberingMap: Map<string, NumberingInfo>,
): DocxNode[] {
  const nodes: DocxNode[] = [];
  let pCount = 0;
  let tblCount = 0;

  for (let i = 0; i < bodyChildren.length; i++) {
    const child = bodyChildren[i];

    if ("w:p" in child) {
      const id = idPrefix ? `${idPrefix}_p${pCount}` : `p_${pCount}`;
      const paragraph = parseParagraph(
        childrenOf(child, "w:p"),
        id,
        rels,
        numberingMap,
      );
      locationMap.set(id, {
        parentPath: idPrefix || "body",
        index: i,
        tag: "w:p",
      });
      nodes.push(paragraph);
      pCount++;
    } else if ("w:tbl" in child) {
      const id = idPrefix ? `${idPrefix}_tbl${tblCount}` : `tbl_${tblCount}`;
      const table = parseTable(
        childrenOf(child, "w:tbl"),
        id,
        locationMap,
        rels,
        numberingMap,
      );
      locationMap.set(id, {
        parentPath: idPrefix || "body",
        index: i,
        tag: "w:tbl",
      });
      nodes.push(table);
      tblCount++;
    }
    // Skip w:sectPr, w:tcPr, w:tblPr, etc.
  }

  return nodes;
}

// ─── Paragraph Parsing ───────────────────────────────────────────────

function parseParagraph(
  pChildren: XmlObj[],
  id: string,
  rels: Map<string, string>,
  numberingMap: Map<string, NumberingInfo>,
): DocxParagraph {
  let style: string | undefined;
  let numbering: DocxParagraph["numbering"];
  const runs: DocxRun[] = [];

  for (const item of pChildren) {
    if ("w:pPr" in item) {
      const ppr = parseParagraphProperties(
        childrenOf(item, "w:pPr"),
        numberingMap,
      );
      style = ppr.style;
      numbering = ppr.numbering;
    } else if ("w:r" in item) {
      runs.push(parseRun(childrenOf(item, "w:r")));
    } else if ("w:hyperlink" in item) {
      const hlRuns = parseHyperlink(item, rels);
      runs.push(...hlRuns);
    }
  }

  return { type: "paragraph", id, runs, style, numbering };
}

function parseParagraphProperties(
  pprChildren: XmlObj[],
  numberingMap: Map<string, NumberingInfo>,
): { style?: string; numbering?: DocxParagraph["numbering"] } {
  let style: string | undefined;
  let numbering: DocxParagraph["numbering"];

  for (const item of pprChildren) {
    if ("w:pStyle" in item) {
      style = attrVal(item, "@_w:val");
    }

    if ("w:numPr" in item) {
      const numPrChildren = childrenOf(item, "w:numPr");
      let numId: string | undefined;
      let level = 0;

      const ilvlObj = findChild(numPrChildren, "w:ilvl");
      if (ilvlObj) level = parseInt(attrVal(ilvlObj, "@_w:val") ?? "0", 10);

      const numIdObj = findChild(numPrChildren, "w:numId");
      if (numIdObj) numId = attrVal(numIdObj, "@_w:val");

      if (numId) {
        const numInfo = numberingMap.get(numId);
        const levelInfo = numInfo?.levels.get(level);
        numbering = { numId, level, format: levelInfo?.format };
      }
    }
  }

  return { style, numbering };
}

// ─── Run Parsing ─────────────────────────────────────────────────────

function parseRun(rChildren: XmlObj[]): DocxRun {
  let bold = false;
  let italic = false;
  let underline = false;
  let strike = false;
  let runStyle: string | undefined;
  let text = "";

  for (const item of rChildren) {
    if ("w:rPr" in item) {
      const rprChildren = childrenOf(item, "w:rPr");
      for (const prop of rprChildren) {
        if ("w:b" in prop || "w:bCs" in prop) bold = true;
        if ("w:i" in prop || "w:iCs" in prop) italic = true;
        if ("w:u" in prop) underline = true;
        if ("w:strike" in prop) strike = true;
        if ("w:rStyle" in prop) {
          runStyle = attrVal(prop, "@_w:val");
        }
      }
    } else if ("w:t" in item) {
      const tChildren = childrenOf(item, "w:t");
      text += extractText(tChildren);
    }
  }

  return {
    text,
    ...(bold && { bold }),
    ...(italic && { italic }),
    ...(underline && { underline }),
    ...(strike && { strike }),
    ...(runStyle && { style: runStyle }),
  };
}

// ─── Hyperlink Parsing ───────────────────────────────────────────────

function parseHyperlink(hlObj: XmlObj, rels: Map<string, string>): DocxRun[] {
  const runs: DocxRun[] = [];

  // The r:id is on the hyperlink element's `:@` attributes
  const rId = attrVal(hlObj, "@_r:id");
  const url = rId ? rels.get(rId) : undefined;

  const hlChildren = childrenOf(hlObj, "w:hyperlink");
  for (const child of hlChildren) {
    if ("w:r" in child) {
      const run = parseRun(childrenOf(child, "w:r"));
      if (url) run.hyperlink = url;
      runs.push(run);
    }
  }

  return runs;
}

// ─── Table Parsing ───────────────────────────────────────────────────

function parseTable(
  tblChildren: XmlObj[],
  tableId: string,
  locationMap: Map<string, XmlElementLocation>,
  rels: Map<string, string>,
  numberingMap: Map<string, NumberingInfo>,
): DocxTable {
  const rows: DocxTableRow[] = [];
  let rowIndex = 0;

  for (const child of tblChildren) {
    if ("w:tr" in child) {
      const row = parseTableRow(
        childrenOf(child, "w:tr"),
        tableId,
        rowIndex,
        locationMap,
        rels,
        numberingMap,
      );
      rows.push(row);
      rowIndex++;
    }
  }

  return { type: "table", id: tableId, rows };
}

function parseTableRow(
  trChildren: XmlObj[],
  tableId: string,
  rowIndex: number,
  locationMap: Map<string, XmlElementLocation>,
  rels: Map<string, string>,
  numberingMap: Map<string, NumberingInfo>,
): DocxTableRow {
  const cells: DocxTableCell[] = [];
  let cellIndex = 0;

  for (const child of trChildren) {
    if ("w:tc" in child) {
      const cellId = `${tableId}_r${rowIndex}_c${cellIndex}`;
      const cell = parseTableCell(
        childrenOf(child, "w:tc"),
        cellId,
        locationMap,
        rels,
        numberingMap,
      );
      cells.push(cell);
      cellIndex++;
    }
  }

  return { cells };
}

function parseTableCell(
  tcChildren: XmlObj[],
  cellId: string,
  locationMap: Map<string, XmlElementLocation>,
  rels: Map<string, string>,
  numberingMap: Map<string, NumberingInfo>,
): DocxTableCell {
  let gridSpan: number | undefined;

  // Check for cell properties
  const tcPrObj = findChild(tcChildren, "w:tcPr");
  if (tcPrObj) {
    const tcPrChildren = childrenOf(tcPrObj, "w:tcPr");
    const gsObj = findChild(tcPrChildren, "w:gridSpan");
    if (gsObj) {
      gridSpan = parseInt(attrVal(gsObj, "@_w:val") ?? "1", 10);
    }
  }

  // Parse cell content (paragraphs, nested tables)
  const children = walkBodyChildren(
    tcChildren,
    cellId,
    locationMap,
    rels,
    numberingMap,
  );

  return { id: cellId, children, gridSpan };
}

// ─── Text Extraction ─────────────────────────────────────────────────

function extractText(tChildren: XmlObj[]): string {
  let text = "";
  for (const item of tChildren) {
    if ("#text" in item) {
      text += String(item["#text"]);
    }
  }
  return text;
}
