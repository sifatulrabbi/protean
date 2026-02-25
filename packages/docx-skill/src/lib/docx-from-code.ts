import { execFile } from "node:child_process";
import path from "node:path";
import { promisify } from "node:util";
import { type FS } from "@protean/vfs";

const execFileAsync = promisify(execFile);

/**
 * Generate a DOCX file by executing user-provided docxjs code.
 *
 * The caller supplies TypeScript code that must define a `doc` variable
 * (a `docx.Document` instance). This function wraps that code in a
 * template that imports the `docx` package, packs the document to a
 * buffer, and writes it to `outputPath`.
 */
export async function generateDocxFromCode(
  code: string,
  outputPath: string,
  fsClient: FS,
): Promise<{ outputPath: string }> {
  const absOutputPath = fsClient.resolvePath(outputPath);
  const safeOutputPath = JSON.stringify(absOutputPath);

  const script = `import * as docx from "docx";
const { Document, Packer, Paragraph, TextRun, HeadingLevel, Table, TableRow, TableCell, WidthType, AlignmentType, ExternalHyperlink, ImageRun, BorderStyle, ShadingType, TabStopPosition, TabStopType, NumberFormat, PageBreak, Header, Footer } = docx;

// --- USER CODE ---
${code}
// --- END USER CODE ---

if (typeof doc === "undefined") {
  throw new Error("User code must define a 'doc' variable (a docx.Document instance).");
}

const buffer = await Packer.toBuffer(doc);
await Bun.write(${safeOutputPath}, buffer);
`;

  // Ensure the output directory exists in the workspace
  await fsClient.mkdir(path.dirname(outputPath));

  // Write temp script into the workspace
  const scriptName = `_docx-gen-${Date.now()}.ts`;
  const scriptRelPath = path.join(path.dirname(outputPath), scriptName);
  await fsClient.writeFile(scriptRelPath, script);

  const absScriptPath = fsClient.resolvePath(scriptRelPath);

  try {
    await execFileAsync("bun", ["run", absScriptPath], {
      timeout: 30_000,
    });
  } catch (err: any) {
    const stderr = err.stderr?.trim() || err.message || "Unknown error";
    throw new Error(`DOCX code generation failed:\n${stderr}`);
  } finally {
    // Always clean up temp script
    await fsClient.remove(scriptRelPath).catch(() => {});
  }

  // Validate output exists
  const fileStat = await fsClient.stat(outputPath);
  if (fileStat.size === 0) {
    throw new Error(
      `DOCX generation produced an empty file at "${outputPath}".`,
    );
  }

  return { outputPath };
}
