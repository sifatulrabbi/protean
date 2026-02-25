/**
 * Internal types for the DOCX parser.
 * These represent the parsed document tree with assigned element IDs.
 */

export interface DocxRun {
  text: string;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  strike?: boolean;
  hyperlink?: string; // URL if this run is inside a hyperlink
  style?: string; // e.g. "Hyperlink"
}

export interface DocxParagraph {
  type: "paragraph";
  id: string;
  runs: DocxRun[];
  style?: string; // e.g. "Heading1", "Heading2", "ListParagraph"
  numbering?: {
    numId: string;
    level: number;
    format?: string; // "decimal", "bullet", etc.
  };
}

export interface DocxTableCell {
  id: string;
  children: DocxNode[];
  gridSpan?: number; // for merged cells
}

export interface DocxTableRow {
  cells: DocxTableCell[];
}

export interface DocxTable {
  type: "table";
  id: string;
  rows: DocxTableRow[];
}

export type DocxNode = DocxParagraph | DocxTable;

/**
 * Tracks the location of an element in the raw XML structure
 * so we can find it again during modification.
 */
export interface XmlElementLocation {
  /** Path to the parent array in the parsed XML object */
  parentPath: string;
  /** Index within that parent array */
  index: number;
  /** The element type tag (e.g. "w:p", "w:tbl") */
  tag: string;
}

/**
 * Result of parsing a DOCX file.
 */
export interface ParsedDocx {
  /** The parsed document tree */
  nodes: DocxNode[];
  /** Map from element ID to its XML location (for modifications) */
  locationMap: Map<string, XmlElementLocation>;
  /** Relationship map: rId → target (URLs, image paths, etc.) */
  rels: Map<string, string>;
  /** The raw JSZip instance for repackaging */
  zip: import("jszip");
  /** The parsed XML object (for modification) */
  parsedXml: Record<string, unknown>[];
  /** Numbering definitions: numId → abstractNum format info */
  numberingMap: Map<string, NumberingInfo>;
}

export interface NumberingInfo {
  levels: Map<number, { format: string; text: string }>;
}
