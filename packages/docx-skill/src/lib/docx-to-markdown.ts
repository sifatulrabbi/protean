/**
 * Converts a parsed DocxNode[] tree into Markdown with element ID comments.
 */
import type {
  DocxNode,
  DocxParagraph,
  DocxTable,
  DocxTableCell,
  DocxRun,
} from "./types";

// ─── Main Entry ──────────────────────────────────────────────────────

export function nodesToMarkdown(
  nodes: DocxNode[],
  rels: Map<string, string>,
): string {
  const lines: string[] = [];

  for (const node of nodes) {
    if (node.type === "paragraph") {
      const md = paragraphToMarkdown(node);
      if (md !== null) {
        lines.push(`<!-- ${node.id} -->`);
        lines.push(md);
        lines.push("");
      }
    } else if (node.type === "table") {
      lines.push(`<!-- ${node.id} -->`);
      lines.push(...tableToMarkdown(node));
      lines.push("");
    }
  }

  return lines.join("\n").trimEnd() + "\n";
}

// ─── Paragraph ───────────────────────────────────────────────────────

function paragraphToMarkdown(p: DocxParagraph): string | null {
  const text = runsToMarkdown(p.runs);

  // Skip empty paragraphs
  if (!text.trim()) return null;

  // Heading
  const headingMatch = p.style?.match(/^Heading(\d)$/);
  if (headingMatch) {
    const level = parseInt(headingMatch[1], 10);
    const prefix = "#".repeat(level);
    return `${prefix} ${text}`;
  }

  // Numbered list
  if (p.numbering) {
    if (p.numbering.format === "decimal") {
      const indent = "  ".repeat(p.numbering.level);
      return `${indent}1. ${text}`;
    }
    // Bullet list
    const indent = "  ".repeat(p.numbering.level);
    return `${indent}- ${text}`;
  }

  return text;
}

// ─── Runs ────────────────────────────────────────────────────────────

function runsToMarkdown(runs: DocxRun[]): string {
  // Group consecutive hyperlink runs with the same URL
  const segments: string[] = [];
  let i = 0;

  while (i < runs.length) {
    const run = runs[i];

    if (run.hyperlink) {
      // Collect all consecutive runs with the same hyperlink URL
      const url = run.hyperlink;
      let linkText = "";
      while (i < runs.length && runs[i].hyperlink === url) {
        linkText += formatRun(runs[i], true);
        i++;
      }
      segments.push(`[${linkText}](${url})`);
    } else {
      segments.push(formatRun(run, false));
      i++;
    }
  }

  return segments.join("");
}

function formatRun(run: DocxRun, insideLink: boolean): string {
  let text = run.text;
  if (!text) return "";

  if (run.strike) text = `~~${text}~~`;
  if (run.underline && !insideLink) text = `<u>${text}</u>`;
  if (run.bold && run.italic) text = `***${text}***`;
  else if (run.bold) text = `**${text}**`;
  else if (run.italic) text = `*${text}*`;

  return text;
}

// ─── Table ───────────────────────────────────────────────────────────

function tableToMarkdown(table: DocxTable): string[] {
  if (table.rows.length === 0) return [];

  const rows = table.rows.map((row) =>
    row.cells.map((cell) => cellToMarkdown(cell)),
  );

  // Determine column count from the widest row
  const colCount = Math.max(...rows.map((r) => r.length));

  // Pad rows to have equal columns
  for (const row of rows) {
    while (row.length < colCount) row.push("");
  }

  const lines: string[] = [];

  // Header row
  lines.push("| " + rows[0].map((c) => c || " ").join(" | ") + " |");
  // Separator
  lines.push("| " + rows[0].map(() => "---").join(" | ") + " |");
  // Data rows
  for (let i = 1; i < rows.length; i++) {
    lines.push("| " + rows[i].map((c) => c || " ").join(" | ") + " |");
  }

  return lines;
}

function cellToMarkdown(cell: DocxTableCell): string {
  // Join paragraphs within cell with space (cells are single-line in MD tables)
  const parts: string[] = [];
  for (const node of cell.children) {
    if (node.type === "paragraph") {
      const text = runsToMarkdown(node.runs);
      if (text.trim()) parts.push(text.trim());
    }
  }
  return parts.join(" ");
}
