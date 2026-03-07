import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import type { Tool } from "ai";
import { noopLogger } from "@protean/logger";

import { createFsTools } from "./fs-tools";

async function makeTempDir() {
  return mkdtemp(join(tmpdir(), "bash-agent-fs-tools-"));
}

async function callTool<T>(toolDef: Tool, input: unknown): Promise<T> {
  if (!toolDef.execute) {
    throw new Error("Tool is missing an execute handler.");
  }

  return (await toolDef.execute(input as never, {} as never)) as T;
}

describe("createFsTools", () => {
  let workspaceRoot = "";

  beforeEach(async () => {
    workspaceRoot = await makeTempDir();
  });

  afterEach(async () => {
    if (workspaceRoot) {
      await rm(workspaceRoot, { recursive: true, force: true });
    }
  });

  test("rejects path traversal attempts", async () => {
    const tools = await createFsTools({ workspaceRoot }, noopLogger);

    try {
      await callTool(tools.ReadFile, {
        path: "../outside.txt",
        withLineNumbers: false,
      });
      throw new Error("Expected path traversal to throw.");
    } catch (error) {
      expect(error).toBeInstanceOf(Error);
      expect((error as Error).message).toMatch(/escapes workspace root/);
    }
  });

  test("reads a file by line range", async () => {
    await writeFile(join(workspaceRoot, "notes.txt"), "one\ntwo\nthree\nfour");
    const tools = await createFsTools({ workspaceRoot }, noopLogger);

    const result = await callTool<{
      ok: boolean;
      content: string;
      startLine: number;
      endLine: number;
    }>(tools.ReadFile, {
      path: "notes.txt",
      startLine: 2,
      endLine: 3,
      withLineNumbers: true,
    });

    expect(result.ok).toBe(true);
    expect(result.startLine).toBe(2);
    expect(result.endLine).toBe(3);
    expect(result.content).toBe("2: two\n3: three");
  });

  test("edits a file with an exact replacement", async () => {
    await writeFile(join(workspaceRoot, "notes.txt"), "alpha beta gamma");
    const tools = await createFsTools({ workspaceRoot }, noopLogger);

    const result = await callTool<{
      ok: boolean;
      replaced: boolean;
      replacements: number;
    }>(tools.EditFile, {
      path: "notes.txt",
      oldString: "beta",
      newString: "delta",
      expectedOccurrences: 1,
    });

    expect(result.ok).toBe(true);
    expect(result.replaced).toBe(true);
    expect(result.replacements).toBe(1);
    expect(await readFile(join(workspaceRoot, "notes.txt"), "utf8")).toBe(
      "alpha delta gamma",
    );
  });

  test("rejects EditFile when expected occurrences do not match", async () => {
    await writeFile(join(workspaceRoot, "notes.txt"), "repeat repeat");
    const tools = await createFsTools({ workspaceRoot }, noopLogger);

    const result = await callTool<{
      ok: boolean;
      replaced: boolean;
      error: string;
    }>(tools.EditFile, {
      path: "notes.txt",
      oldString: "repeat",
      newString: "done",
      replaceAll: true,
      expectedOccurrences: 1,
    });

    expect(result.ok).toBe(false);
    expect(result.replaced).toBe(false);
    expect(result.error).toMatch(/Expected 1 replacements but found 2/);
  });

  test("refuses to overwrite an existing file without overwrite=true", async () => {
    await writeFile(join(workspaceRoot, "existing.txt"), "first");
    const tools = await createFsTools({ workspaceRoot }, noopLogger);

    const result = await callTool<{
      ok: boolean;
      created: boolean;
      error: string;
    }>(tools.CreateEntity, {
      path: "existing.txt",
      kind: "file",
      content: "second",
      overwrite: false,
    });

    expect(result.ok).toBe(false);
    expect(result.created).toBe(false);
    expect(result.error).toMatch(/already exists/);
    expect(await readFile(join(workspaceRoot, "existing.txt"), "utf8")).toBe(
      "first",
    );
  });

  test("returns bounded glob matches under the workspace root", async () => {
    await writeFile(join(workspaceRoot, "one.ts"), "export const one = 1;\n");
    await writeFile(join(workspaceRoot, "two.ts"), "export const two = 2;\n");
    await writeFile(join(workspaceRoot, "three.md"), "# doc\n");
    const tools = await createFsTools({ workspaceRoot }, noopLogger);

    const result = await callTool<{
      ok: boolean;
      matches: string[];
    }>(tools.Glob, {
      pattern: "*.ts",
      includeDirectories: false,
      maxResults: 1,
    });

    expect(result.ok).toBe(true);
    expect(result.matches.length).toBe(1);
    expect(result.matches[0]).toMatch(/\.ts$/);
    expect(result.matches[0]?.includes("..")).toBe(false);
  });
});
