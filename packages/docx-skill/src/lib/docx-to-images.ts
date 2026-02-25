import { execFile } from "node:child_process";
import path from "node:path";
import { promisify } from "node:util";
import { type FS } from "@protean/vfs";

const execFileAsync = promisify(execFile);

const SOFFICE_PATHS = [
  "soffice",
  "/Applications/LibreOffice.app/Contents/MacOS/soffice",
];

async function findSoffice(): Promise<string> {
  for (const candidate of SOFFICE_PATHS) {
    try {
      await execFileAsync(candidate, ["--version"]);
      return candidate;
    } catch {
      // try next
    }
  }
  throw new Error(
    "LibreOffice is required for DOCX to image conversion. Install with: brew install --cask libreoffice",
  );
}

/**
 * Convert a DOCX file to PNG images (one per page).
 *
 * Pipeline: DOCX → PDF (via LibreOffice headless) → PNG per page (via pdf-to-img)
 *
 * @param docxPath - Workspace-relative path to the input DOCX file
 * @param outputDir - Workspace-relative path for the output images
 * @param fsClient - VFS instance for all file operations
 */
export async function docxToImages(
  docxPath: string,
  outputDir: string,
  fsClient: FS,
): Promise<{ pages: string[] }> {
  const sofficePath = await findSoffice();

  // Ensure output directory exists
  await fsClient.mkdir(outputDir);

  // Resolve workspace-relative paths to absolute for external tools
  const absDocxPath = fsClient.resolvePath(docxPath);
  const absOutputDir = fsClient.resolvePath(outputDir);

  // Step 1: Convert DOCX → PDF via LibreOffice headless
  await execFileAsync(sofficePath, [
    "--headless",
    "--convert-to",
    "pdf",
    "--outdir",
    absOutputDir,
    absDocxPath,
  ]);

  // Find the generated PDF (workspace-relative)
  const baseName = path.basename(docxPath, path.extname(docxPath));
  const pdfRelPath = path.join(outputDir, `${baseName}.pdf`);
  const absPdfPath = fsClient.resolvePath(pdfRelPath);

  // Step 2: Convert PDF → PNG images using pdf-to-img
  const { pdf } = await import("pdf-to-img");

  const pages: string[] = [];
  let pageNum = 0;
  for await (const image of await pdf(absPdfPath, { scale: 2.0 })) {
    pageNum++;
    const pageRelPath = path.join(outputDir, `page-${pageNum}.png`);
    await fsClient.writeFileBuffer(pageRelPath, Buffer.from(image));
    pages.push(pageRelPath);
  }

  return { pages };
}
