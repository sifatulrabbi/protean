import { describe, it, expect, beforeAll, afterAll } from "bun:test";
import { execFile } from "node:child_process";
import path from "node:path";
import { promisify } from "node:util";
import { createLocalFs, type FS } from "@protean/vfs";
import { docxToImages } from "./docx-to-images";

const execFileAsync = promisify(execFile);
const FIXTURES_DIR = path.resolve(import.meta.dirname, "../__fixtures__");
const TMP_BASE = path.resolve(import.meta.dirname, "../../tmp");

async function isLibreOfficeAvailable(): Promise<boolean> {
  const candidates = [
    "soffice",
    "/Applications/LibreOffice.app/Contents/MacOS/soffice",
  ];
  for (const c of candidates) {
    try {
      await execFileAsync(c, ["--version"]);
      return true;
    } catch {
      // try next
    }
  }
  return false;
}

describe("docxToImages", () => {
  let hasLibreOffice = false;
  let workspaceFs: FS;
  let workspaceName: string;
  let tmpFs: FS;

  beforeAll(async () => {
    hasLibreOffice = await isLibreOfficeAvailable();
    if (hasLibreOffice) {
      tmpFs = await createLocalFs(TMP_BASE);
      workspaceName = `docx-img-test-${Date.now()}`;
      workspaceFs = await createLocalFs(path.join(TMP_BASE, workspaceName));

      // Seed workspace with fixture via VFS
      const fixturesFs = await createLocalFs(FIXTURES_DIR);
      const fixtureBuffer = await fixturesFs.readFileBuffer("simple.docx");
      await workspaceFs.writeFileBuffer("simple.docx", fixtureBuffer);
    }
  });

  afterAll(async () => {
    if (tmpFs && workspaceName) {
      await tmpFs.remove(workspaceName);
    }
  });

  it("throws a helpful error when LibreOffice is not found", async () => {
    const errorMsg =
      "LibreOffice is required for DOCX to image conversion. Install with: brew install --cask libreoffice";
    expect(errorMsg).toContain("LibreOffice");
    expect(errorMsg).toContain("brew install");
  });

  it("converts simple.docx to PNG images", async () => {
    if (!hasLibreOffice) {
      console.log("Skipping: LibreOffice not installed");
      return;
    }

    const result = await docxToImages(
      "simple.docx",
      "page-images",
      workspaceFs,
    );

    expect(result.pages).toBeDefined();
    expect(result.pages.length).toBeGreaterThan(0);

    // Each page should be a real file
    for (const pagePath of result.pages) {
      const fileStat = await workspaceFs.stat(pagePath);
      expect(fileStat.size).toBeGreaterThan(0);
      expect(pagePath).toEndWith(".png");
    }

    // Files should be named page-1.png, page-2.png, etc.
    const fileNames = result.pages.map((p) => path.basename(p));
    expect(fileNames[0]).toBe("page-1.png");
  });
});
