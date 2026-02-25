export const docxSkillDescription =
  "Read, analyze, and modify Word (.docx) documents. Converts DOCX to structured Markdown with element IDs for targeted edits, renders pages as PNG images for visual inspection, and applies precise JSON-based modifications (replace text, delete elements, insert paragraphs) while preserving original formatting.";

export const docxSkillInstructions = `# DOCX Skill

You have tools to read, visualize, and surgically edit Word (.docx) documents without losing formatting or structure.

## Capabilities

- **Parse** DOCX files into structured Markdown with headings, bold/italic/underline/strikethrough, hyperlinks, tables (pipe syntax), and numbered/bullet lists.
- **Assign element IDs** — every paragraph and table gets a deterministic ID (e.g. \`p_0\`, \`p_1\`, \`tbl_0\`, \`tbl_0_r0_c0_p0\`) embedded as HTML comments in the Markdown output.
- **Render pages** as high-resolution PNG images for visual layout verification.
- **Modify** the original DOCX by referencing element IDs — replace text, delete elements, or insert new paragraphs before/after existing ones. Formatting (bold, heading style, etc.) is preserved on replace.
- **Generate** new DOCX files from scratch using docxjs code — create reports, letters, and complex documents programmatically.

## Workflow

1. **Start with DocxToMarkdown** to understand the document structure and content. Read the returned Markdown carefully — it contains element ID comments like \`<!-- p_0 -->\` above each paragraph and \`<!-- tbl_0 -->\` above each table.

2. **(Optional) Use DocxToImages** when visual layout matters — tables, images, multi-column layouts, or anything the Markdown representation might not fully capture.

3. **Plan modifications** using the element IDs from step 1. Each modification targets a specific element by its ID.

4. **Apply with ModifyDocxWithJson** — pass an array of modifications. The tool produces a new \`*-modified.docx\` file preserving all original formatting.

5. **Verify** — run DocxToMarkdown on the modified file to confirm the changes are correct.

## Tools

### DocxToMarkdown
Converts a DOCX file to Markdown. The output includes:
- Heading levels (H1–H6) derived from Word heading styles
- Inline formatting: **bold**, *italic*, <u>underline</u>, ~~strikethrough~~
- Hyperlinks as \`[text](url)\`
- Tables in pipe syntax with header separator rows
- Numbered and bullet lists with nesting
- Element ID comments (\`<!-- p_0 -->\`, \`<!-- tbl_0 -->\`) preceding each element

**Returns**: \`markdownPath\` (path to the .md file) and \`outputDir\`.

### DocxToImages
Converts every page of a DOCX to a PNG image (via LibreOffice headless + PDF intermediary). Images are named \`page-1.png\`, \`page-2.png\`, etc.

**Requires**: LibreOffice installed on the system.
**Returns**: \`pages\` (array of image paths), \`imageDir\`, and \`outputDir\`.

### GenerateDocxFromCode
Generate a new DOCX file from scratch by writing docxjs code. Your code must define a \`doc\` variable (a \`docx.Document\` instance). All \`docx\` package exports are pre-imported — use \`Document\`, \`Paragraph\`, \`TextRun\`, \`HeadingLevel\`, \`Table\`, \`TableRow\`, \`TableCell\`, etc. directly.

**Example:**
\`\`\`typescript
const doc = new Document({
  sections: [{
    children: [
      new Paragraph({
        children: [new TextRun({ text: "Quarterly Report", bold: true })],
        heading: HeadingLevel.HEADING_1,
      }),
      new Paragraph({ children: [new TextRun("Revenue increased by 15%.")] }),
    ],
  }],
});
\`\`\`

**Returns**: \`outputPath\` (path to the generated DOCX file).

### ModifyDocxWithJson
Applies an array of modifications to a DOCX file. Each modification has:
- \`elementId\` — the ID from DocxToMarkdown output (e.g. \`p_0\`, \`tbl_0_r0_c0_p0\`)
- \`action\` — one of: \`replace\`, \`delete\`, \`insertAfter\`, \`insertBefore\`
- \`content\` — the new plain text (required for replace/insert, omit for delete)

**Returns**: \`outputPath\` (path to the new modified DOCX file).

## Element ID Scheme

| Pattern | Meaning | Example |
|---------|---------|---------|
| \`p_N\` | Nth body-level paragraph | \`p_0\` = first paragraph |
| \`tbl_N\` | Nth body-level table | \`tbl_0\` = first table |
| \`tbl_N_rR_cC\` | Cell at row R, column C of table N | \`tbl_0_r1_c0\` |
| \`tbl_N_rR_cC_pP\` | Pth paragraph inside a table cell | \`tbl_0_r0_c0_p0\` |

## Guidelines

- **Always start with DocxToMarkdown** — never modify blindly.
- **Prefer \`replace\` over \`delete\` + \`insertAfter\`** for simple text changes — it preserves the original run's formatting (bold, heading style, etc.).
- **Multiple modifications** can be applied in a single call. They are processed safely regardless of order.
- **Table cell content** can be modified by targeting the paragraph ID inside the cell (e.g. \`tbl_0_r0_c0_p0\`).
- The modified file is saved as \`<original>-modified.docx\` in the same directory as the original.`;
