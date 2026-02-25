/**
 * Run with: bun src/__fixtures__/generate-fixtures.ts
 * Generates test .docx fixtures for unit/integration tests.
 */
import {
  Document,
  Packer,
  Paragraph,
  TextRun,
  HeadingLevel,
  Table,
  TableRow,
  TableCell,
  WidthType,
  ImageRun,
  AlignmentType,
  ExternalHyperlink,
} from "docx";
import { writeFile } from "node:fs/promises";
import path from "node:path";

const FIXTURES_DIR = path.dirname(new URL(import.meta.url).pathname);

async function generateSimpleDoc() {
  const doc = new Document({
    numbering: {
      config: [
        {
          reference: "numbered-list",
          levels: [
            {
              level: 0,
              format: "decimal" as any,
              text: "%1.",
              alignment: AlignmentType.START,
            },
          ],
        },
      ],
    },
    sections: [
      {
        children: [
          // Heading 1
          new Paragraph({
            heading: HeadingLevel.HEADING_1,
            children: [new TextRun("Introduction")],
          }),

          // Normal paragraph with bold + italic
          new Paragraph({
            children: [
              new TextRun("This is a "),
              new TextRun({ text: "bold", bold: true }),
              new TextRun(" and "),
              new TextRun({ text: "italic", italics: true }),
              new TextRun(" paragraph."),
            ],
          }),

          // Heading 2
          new Paragraph({
            heading: HeadingLevel.HEADING_2,
            children: [new TextRun("Details")],
          }),

          // Paragraph with hyperlink
          new Paragraph({
            children: [
              new TextRun("Visit "),
              new ExternalHyperlink({
                children: [
                  new TextRun({
                    text: "Example Site",
                    style: "Hyperlink",
                  }),
                ],
                link: "https://example.com",
              }),
              new TextRun(" for more info."),
            ],
          }),

          // Simple table
          new Table({
            rows: [
              new TableRow({
                children: [
                  new TableCell({
                    width: { size: 3000, type: WidthType.DXA },
                    children: [
                      new Paragraph({
                        children: [new TextRun({ text: "Name", bold: true })],
                      }),
                    ],
                  }),
                  new TableCell({
                    width: { size: 3000, type: WidthType.DXA },
                    children: [
                      new Paragraph({
                        children: [new TextRun({ text: "Age", bold: true })],
                      }),
                    ],
                  }),
                ],
              }),
              new TableRow({
                children: [
                  new TableCell({
                    width: { size: 3000, type: WidthType.DXA },
                    children: [new Paragraph("Alice")],
                  }),
                  new TableCell({
                    width: { size: 3000, type: WidthType.DXA },
                    children: [new Paragraph("30")],
                  }),
                ],
              }),
              new TableRow({
                children: [
                  new TableCell({
                    width: { size: 3000, type: WidthType.DXA },
                    children: [new Paragraph("Bob")],
                  }),
                  new TableCell({
                    width: { size: 3000, type: WidthType.DXA },
                    children: [new Paragraph("25")],
                  }),
                ],
              }),
            ],
          }),

          // Paragraph after table
          new Paragraph({
            children: [new TextRun("This is the final paragraph.")],
          }),

          // Numbered list
          new Paragraph({
            heading: HeadingLevel.HEADING_2,
            children: [new TextRun("Action Items")],
          }),
          new Paragraph({
            numbering: { reference: "numbered-list", level: 0 },
            children: [new TextRun("First item")],
          }),
          new Paragraph({
            numbering: { reference: "numbered-list", level: 0 },
            children: [new TextRun("Second item")],
          }),
          new Paragraph({
            numbering: { reference: "numbered-list", level: 0 },
            children: [new TextRun("Third item")],
          }),
        ],
      },
    ],
  });

  const buffer = await Packer.toBuffer(doc);
  const outPath = path.join(FIXTURES_DIR, "simple.docx");
  await writeFile(outPath, buffer);
  console.log(`Written: ${outPath}`);
}

async function main() {
  await generateSimpleDoc();
  console.log("All fixtures generated.");
}

main().catch(console.error);
