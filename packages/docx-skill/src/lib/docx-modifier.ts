/**
 * DOCX modifier — applies modifications to a parsed DOCX document.
 *
 * Uses the locationMap from ParsedDocx to find elements by ID,
 * manipulates the preserveOrder XML tree, and repackages the ZIP.
 */
import type { ParsedDocx, XmlElementLocation } from "./types";
import type { DocxModification } from "../converter";
import { xmlBuilder } from "./docx-parser";

type XmlObj = Record<string, unknown>;

// ─── PreserveOrder Helpers (mirrored from docx-parser) ──────────────

function tagOf(obj: XmlObj): string | undefined {
  for (const key of Object.keys(obj)) {
    if (key !== ":@" && key !== "#text") return key;
  }
  return undefined;
}

function childrenOf(obj: XmlObj, tag: string): XmlObj[] {
  const val = obj[tag];
  if (Array.isArray(val)) return val as XmlObj[];
  return [];
}

function findChild(children: XmlObj[], tag: string): XmlObj | undefined {
  return children.find((c) => tag in c);
}

// ─── Navigation ─────────────────────────────────────────────────────

/**
 * Resolve the parent children array that contains the element at `location.index`.
 *
 * For body-level elements (parentPath === "body"), we navigate:
 *   parsedXml → w:document children → w:body children
 *
 * For table cell elements (parentPath like "tbl_0_r0_c0"), we first
 * resolve the body children, find the table, then drill into the row/cell.
 */
function resolveParentArray(parsedXml: XmlObj[], parentPath: string): XmlObj[] {
  // Get body children
  const docObj = parsedXml.find((item) => "w:document" in item);
  if (!docObj) throw new Error("Cannot find w:document in parsedXml");
  const docChildren = childrenOf(docObj, "w:document");
  const bodyObj = findChild(docChildren, "w:body");
  if (!bodyObj) throw new Error("Cannot find w:body in parsedXml");
  const bodyChildren = childrenOf(bodyObj, "w:body");

  if (parentPath === "body") {
    return bodyChildren;
  }

  // Parse cell path like "tbl_0_r0_c0" or nested like "tbl_0_r0_c0_tbl_1_r0_c0"
  // We need to navigate: body → find table → find row → find cell → cell children
  return resolveCellChildren(bodyChildren, parentPath);
}

/**
 * Navigate from body children into a table cell, returning that cell's children array.
 * Handles paths like "tbl_0_r0_c0" or deeper nesting.
 */
function resolveCellChildren(
  bodyChildren: XmlObj[],
  cellPath: string,
): XmlObj[] {
  // Match segments: tbl_N, rN, cN (possibly repeated for nested tables)
  const segments = cellPath.split("_");
  let current = bodyChildren;

  let i = 0;
  while (i < segments.length) {
    if (segments[i] === "tbl") {
      // tbl_N → find the Nth w:tbl in current array
      const tblIndex = parseInt(segments[i + 1], 10);
      const tables = current.filter((c) => "w:tbl" in c);
      const tblObj = tables[tblIndex];
      if (!tblObj)
        throw new Error(
          `Cannot find table at index ${tblIndex} in path ${cellPath}`,
        );
      current = childrenOf(tblObj, "w:tbl");
      i += 2;
    } else if (segments[i].startsWith("r")) {
      // rN → find the Nth w:tr
      const rowIndex = parseInt(segments[i].slice(1), 10);
      const rows = current.filter((c) => "w:tr" in c);
      const rowObj = rows[rowIndex];
      if (!rowObj)
        throw new Error(
          `Cannot find row at index ${rowIndex} in path ${cellPath}`,
        );
      current = childrenOf(rowObj, "w:tr");
      i += 1;
    } else if (segments[i].startsWith("c")) {
      // cN → find the Nth w:tc
      const cellIndex = parseInt(segments[i].slice(1), 10);
      const cells = current.filter((c) => "w:tc" in c);
      const cellObj = cells[cellIndex];
      if (!cellObj)
        throw new Error(
          `Cannot find cell at index ${cellIndex} in path ${cellPath}`,
        );
      current = childrenOf(cellObj, "w:tc");
      i += 1;
    } else if (segments[i] === "p" || !isNaN(parseInt(segments[i], 10))) {
      // Skip — these are paragraph indices handled by locationMap.index
      i += 1;
    } else {
      i += 1;
    }
  }

  return current;
}

// ─── Modification Operations ────────────────────────────────────────

/**
 * Replace all text in a w:p element with new content.
 * Keeps the first w:r's formatting, removes other w:r elements.
 */
function applyReplace(
  parentArray: XmlObj[],
  index: number,
  content: string,
): void {
  const element = parentArray[index];
  if (!element) return;

  const tag = tagOf(element);
  if (!tag) return;
  const children = childrenOf(element, tag) as XmlObj[];

  // Find runs (w:r)
  const runIndices: number[] = [];
  for (let i = 0; i < children.length; i++) {
    if ("w:r" in children[i]) {
      runIndices.push(i);
    }
  }

  if (runIndices.length === 0) {
    // No runs — create one
    const newRun: XmlObj = {
      "w:r": [
        { "w:t": [{ "#text": content }], ":@": { "@_xml:space": "preserve" } },
      ],
    };
    children.push(newRun);
    return;
  }

  // Keep the first run, update its text, remove the rest
  const firstRunObj = children[runIndices[0]];
  const firstRunChildren = childrenOf(firstRunObj, "w:r") as XmlObj[];

  // Replace all w:t elements with a single one containing the new text
  // Remove existing w:t elements
  for (let i = firstRunChildren.length - 1; i >= 0; i--) {
    if ("w:t" in firstRunChildren[i]) {
      firstRunChildren.splice(i, 1);
    }
  }
  // Add new w:t
  firstRunChildren.push({
    "w:t": [{ "#text": content }],
    ":@": { "@_xml:space": "preserve" },
  });

  // Remove subsequent runs in reverse order
  for (let i = runIndices.length - 1; i >= 1; i--) {
    children.splice(runIndices[i], 1);
  }
}

/**
 * Delete an element from its parent array.
 */
function applyDelete(parentArray: XmlObj[], index: number): void {
  parentArray.splice(index, 1);
}

/**
 * Create a simple w:p element with the given text content.
 */
function createSimpleParagraph(content: string): XmlObj {
  return {
    "w:p": [
      {
        "w:r": [
          {
            "w:t": [{ "#text": content }],
            ":@": { "@_xml:space": "preserve" },
          },
        ],
      },
    ],
  };
}

/**
 * Insert a new paragraph after the element at the given index.
 */
function applyInsertAfter(
  parentArray: XmlObj[],
  index: number,
  content: string,
): void {
  const newP = createSimpleParagraph(content);
  parentArray.splice(index + 1, 0, newP);
}

/**
 * Insert a new paragraph before the element at the given index.
 */
function applyInsertBefore(
  parentArray: XmlObj[],
  index: number,
  content: string,
): void {
  const newP = createSimpleParagraph(content);
  parentArray.splice(index, 0, newP);
}

// ─── Public API ─────────────────────────────────────────────────────

export async function applyModifications(
  parsedDocx: ParsedDocx,
  modifications: DocxModification[],
): Promise<Buffer> {
  const { parsedXml, locationMap, zip } = parsedDocx;

  // Group modifications by action for ordering.
  // Process deletions last and in reverse index order to avoid index shifting.
  // Process insertions in reverse order too to maintain correct indices.
  const deletes: Array<{ mod: DocxModification; loc: XmlElementLocation }> = [];
  const others: Array<{ mod: DocxModification; loc: XmlElementLocation }> = [];

  for (const mod of modifications) {
    const loc = locationMap.get(mod.elementId);
    if (!loc) {
      throw new Error(`Element ID "${mod.elementId}" not found in locationMap`);
    }
    if (mod.action === "delete") {
      deletes.push({ mod, loc });
    } else {
      others.push({ mod, loc });
    }
  }

  // Apply non-delete modifications first (replace, insertAfter, insertBefore)
  // Process inserts in reverse index order within same parent to keep indices stable
  others.sort((a, b) => b.loc.index - a.loc.index);

  for (const { mod, loc } of others) {
    const parentArray = resolveParentArray(
      parsedXml as XmlObj[],
      loc.parentPath,
    );

    switch (mod.action) {
      case "replace":
        applyReplace(parentArray, loc.index, mod.content ?? "");
        break;
      case "insertAfter":
        applyInsertAfter(parentArray, loc.index, mod.content ?? "");
        break;
      case "insertBefore":
        applyInsertBefore(parentArray, loc.index, mod.content ?? "");
        break;
    }
  }

  // Apply deletions in reverse index order to avoid shifting
  deletes.sort((a, b) => {
    // Sort by parent first, then by index descending
    if (a.loc.parentPath !== b.loc.parentPath) {
      return a.loc.parentPath.localeCompare(b.loc.parentPath);
    }
    return b.loc.index - a.loc.index;
  });

  for (const { loc } of deletes) {
    const parentArray = resolveParentArray(
      parsedXml as XmlObj[],
      loc.parentPath,
    );
    applyDelete(parentArray, loc.index);
  }

  // Serialize back to XML string
  const newXmlStr = xmlBuilder.build(parsedXml);

  // Update the zip
  zip.file("word/document.xml", newXmlStr);

  // Generate new buffer
  const newBuffer = await zip.generateAsync({ type: "nodebuffer" });
  return newBuffer;
}
