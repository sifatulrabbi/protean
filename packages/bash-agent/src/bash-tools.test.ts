import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { mkdir, realpath, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { mkdtemp } from "node:fs/promises";
import type { Tool } from "ai";
import { noopLogger } from "@protean/logger";

import { createBashTools } from "./bash-tools";

async function makeTempDir() {
  return mkdtemp(join(tmpdir(), "bash-agent-bash-tools-"));
}

async function callTool<T>(toolDef: Tool, input: unknown): Promise<T> {
  if (!toolDef.execute) {
    throw new Error("Tool is missing an execute handler.");
  }

  return (await toolDef.execute(input as never, {} as never)) as T;
}

describe("createBashTools", () => {
  let workspaceRoot = "";
  const originalAllowed = process.env.SECRET_ALLOWED;
  const originalBlocked = process.env.SECRET_BLOCKED;

  beforeEach(async () => {
    workspaceRoot = await makeTempDir();
    process.env.SECRET_ALLOWED = "allowed-value";
    process.env.SECRET_BLOCKED = "blocked-value";
  });

  afterEach(async () => {
    if (workspaceRoot) {
      await rm(workspaceRoot, { recursive: true, force: true });
    }

    if (typeof originalAllowed === "string") {
      process.env.SECRET_ALLOWED = originalAllowed;
    } else {
      delete process.env.SECRET_ALLOWED;
    }

    if (typeof originalBlocked === "string") {
      process.env.SECRET_BLOCKED = originalBlocked;
    } else {
      delete process.env.SECRET_BLOCKED;
    }
  });

  test("runs Bash commands in the configured cwd", async () => {
    await mkdir(join(workspaceRoot, "nested"), { recursive: true });
    const tools = await createBashTools(
      {
        workspaceRoot,
        cwd: "nested",
      },
      noopLogger,
    );

    const result = await callTool<{
      ok: boolean;
      cwd: string;
      stdout: string;
    }>(tools.Bash, {
      command: "pwd",
    });

    expect(result.ok).toBe(true);
    expect(result.cwd).toBe("nested");
    expect(result.stdout.trim()).toBe(
      await realpath(join(workspaceRoot, "nested")),
    );
  });

  test("reports timeouts for long-running commands", async () => {
    const tools = await createBashTools(
      {
        workspaceRoot,
        timeoutMs: 25,
      },
      noopLogger,
    );

    const result = await callTool<{
      ok: boolean;
      timedOut: boolean;
    }>(tools.Bash, {
      command: "sleep 1",
    });

    expect(result.ok).toBe(false);
    expect(result.timedOut).toBe(true);
  });

  test("truncates large stdout and stderr output", async () => {
    const tools = await createBashTools(
      {
        workspaceRoot,
        maxOutputBytes: 256,
      },
      noopLogger,
    );

    const result = await callTool<{
      stdout: string;
      stderr: string;
    }>(tools.Bash, {
      command: "printf 'x%.0s' {1..2000}; printf 'y%.0s' {1..2000} >&2",
    });

    expect(result.stdout).toContain("[truncated]");
    expect(result.stderr).toContain("[truncated]");
  });

  test("passes only explicitly allowlisted environment variables", async () => {
    const tools = await createBashTools(
      {
        workspaceRoot,
        allowEnvKeys: ["SECRET_ALLOWED"],
      },
      noopLogger,
    );

    const result = await callTool<{
      ok: boolean;
      stdout: string;
    }>(tools.Bash, {
      command: 'printf \'%s|%s\' "$SECRET_ALLOWED" "$SECRET_BLOCKED"',
    });

    expect(result.ok).toBe(true);
    expect(result.stdout).toBe("allowed-value|");
  });

  test("returns parsed grep matches", async () => {
    await writeFile(
      join(workspaceRoot, "search.txt"),
      "alpha\nneedle line\nomega\n",
    );
    const tools = await createBashTools({ workspaceRoot }, noopLogger);

    const result = await callTool<{
      ok: boolean;
      engine: string;
      matches: Array<{ path: string; line: number; text: string }>;
    }>(tools.Grep, {
      pattern: "needle",
      path: ".",
      caseSensitive: false,
      maxResults: 100,
    });

    expect(result.ok).toBe(true);
    expect(["rg", "grep"]).toContain(result.engine);
    expect(result.matches.length).toBeGreaterThan(0);
    expect(result.matches[0]?.path).toBe("search.txt");
    expect(result.matches[0]?.line).toBe(2);
    expect(result.matches[0]?.text).toContain("needle");
  });
});
