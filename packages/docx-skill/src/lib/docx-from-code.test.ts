import { describe, it, expect, afterAll } from "bun:test";
import path from "node:path";
import { createLocalFs, type FS } from "@protean/vfs";
import { generateDocxFromCode } from "./docx-from-code";

const TMP_BASE = path.resolve(import.meta.dirname, "../../tmp");

describe("generateDocxFromCode", () => {
  let workspaceFs: FS;
  let tmpFs: FS;
  const workspaceName = `docx-gen-test-${Date.now()}`;

  const setup = async () => {
    tmpFs = await createLocalFs(TMP_BASE);
    workspaceFs = await createLocalFs(path.join(TMP_BASE, workspaceName));
  };

  afterAll(async () => {
    if (tmpFs && workspaceName) {
      await tmpFs.remove(workspaceName);
    }
  });

  it("generates a simple doc with one heading", async () => {
    await setup();

    const code = `
const doc = new Document({
  sections: [{
    children: [
      new Paragraph({
        children: [new TextRun({ text: "Hello World", bold: true })],
        heading: HeadingLevel.HEADING_1,
      }),
    ],
  }],
});
`;

    const result = await generateDocxFromCode(code, "output.docx", workspaceFs);

    expect(result.outputPath).toBe("output.docx");
    const fileStat = await workspaceFs.stat("output.docx");
    expect(fileStat.size).toBeGreaterThan(0);
  });

  it("throws on invalid code (syntax error)", async () => {
    await setup();

    const code = `const doc = <<<INVALID>>>`;

    await expect(
      generateDocxFromCode(code, "bad-syntax.docx", workspaceFs),
    ).rejects.toThrow("DOCX code generation failed");
  });

  it("throws when code does not define doc variable", async () => {
    await setup();

    const code = `const notDoc = new Document({ sections: [] });`;

    await expect(
      generateDocxFromCode(code, "no-doc.docx", workspaceFs),
    ).rejects.toThrow("DOCX code generation failed");
  });
});
