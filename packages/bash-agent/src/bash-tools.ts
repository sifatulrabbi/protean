import { join } from "node:path";
import { tool } from "ai";
import type { Logger } from "@protean/logger";
import { z } from "zod";

import {
  resolveWithinWorkspace,
  toWorkspaceRelativePath,
} from "./workspace-paths";

export interface BashToolsOptions {
  workspaceRoot: string;
  cwd?: string;
  timeoutMs?: number;
  maxOutputBytes?: number;
  allowEnvKeys?: string[];
}

interface CommandResult {
  exitCode: number;
  stdout: string;
  stderr: string;
  timedOut: boolean;
  signalCode: string | null;
}

function truncateText(
  value: string,
  maxOutputBytes: number,
): { text: string; truncated: boolean } {
  const byteLength = Buffer.byteLength(value, "utf8");
  if (byteLength <= maxOutputBytes) {
    return {
      text: value,
      truncated: false,
    };
  }

  let sliceEnd = Math.min(value.length, maxOutputBytes);
  while (
    sliceEnd > 0 &&
    Buffer.byteLength(value.slice(0, sliceEnd), "utf8") > maxOutputBytes
  ) {
    sliceEnd -= 1;
  }

  return {
    text: `${value.slice(0, sliceEnd)}\n[truncated]`,
    truncated: true,
  };
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

function buildEnv(allowEnvKeys: string[]): Record<string, string> {
  const env: Record<string, string> = {};
  const alwaysAllowed = ["PATH", "HOME", "TMPDIR", "SHELL"];

  for (const key of [...alwaysAllowed, ...allowEnvKeys]) {
    const value = process.env[key];
    if (typeof value === "string") {
      env[key] = value;
    }
  }

  return env;
}

function resolveCommandCwd(
  workspaceRoot: string,
  baseCwd: string,
  commandCwd?: string,
): string {
  const resolvedBase = resolveWithinWorkspace(workspaceRoot, baseCwd);
  return resolveWithinWorkspace(
    workspaceRoot,
    commandCwd ? join(resolvedBase, commandCwd) : resolvedBase,
  );
}

async function readStream(stream: ReadableStream<Uint8Array> | null) {
  if (!stream) {
    return "";
  }

  return new Response(stream).text();
}

async function runCommand(
  command: string,
  opts: {
    cwd: string;
    timeoutMs: number;
    maxOutputBytes: number;
    allowEnvKeys: string[];
  },
): Promise<CommandResult> {
  const controller = new AbortController();
  let timedOut = false;

  const timeout = setTimeout(() => {
    timedOut = true;
    controller.abort("timeout");
  }, opts.timeoutMs);

  const proc = Bun.spawn({
    cmd: ["bash", "-lc", command],
    cwd: opts.cwd,
    stdout: "pipe",
    stderr: "pipe",
    env: buildEnv(opts.allowEnvKeys),
    signal: controller.signal,
  });

  try {
    const [stdout, stderr] = await Promise.all([
      readStream(proc.stdout),
      readStream(proc.stderr),
    ]);

    const exitCode = await proc.exited;
    const truncatedStdout = truncateText(stdout, opts.maxOutputBytes);
    const truncatedStderr = truncateText(stderr, opts.maxOutputBytes);

    return {
      exitCode,
      stdout: truncatedStdout.text,
      stderr: truncatedStderr.text,
      timedOut,
      signalCode: proc.signalCode ?? null,
    };
  } finally {
    clearTimeout(timeout);
  }
}

function parseRipgrepJson(output: string, workspaceRoot: string) {
  const matches: Array<{
    path: string;
    line: number;
    column: number | null;
    text: string;
  }> = [];

  for (const line of output.split("\n")) {
    if (!line.trim()) {
      continue;
    }

    let parsed: unknown;
    try {
      parsed = JSON.parse(line);
    } catch {
      continue;
    }

    if (
      typeof parsed !== "object" ||
      parsed === null ||
      !("type" in parsed) ||
      (parsed as { type: unknown }).type !== "match"
    ) {
      continue;
    }

    const data = (
      parsed as {
        data?: {
          path?: { text?: string };
          line_number?: number;
          lines?: { text?: string };
          submatches?: Array<{ start?: number }>;
        };
      }
    ).data;

    if (!data?.path?.text || typeof data.line_number !== "number") {
      continue;
    }

    const absolutePath = resolveWithinWorkspace(workspaceRoot, data.path.text);
    matches.push({
      path: toWorkspaceRelativePath(workspaceRoot, absolutePath),
      line: data.line_number,
      column:
        typeof data.submatches?.[0]?.start === "number"
          ? data.submatches[0].start + 1
          : null,
      text: (data.lines?.text ?? "").replace(/\n$/, ""),
    });
  }

  return matches;
}

function parseGrepLines(output: string, workspaceRoot: string) {
  const matches: Array<{
    path: string;
    line: number;
    column: number | null;
    text: string;
  }> = [];

  for (const line of output.split("\n")) {
    const match = line.match(/^(.*?):(\d+):(.*)$/);
    if (!match) {
      continue;
    }

    const [, filePath, lineNumber, text] = match;
    const absolutePath = resolveWithinWorkspace(workspaceRoot, filePath);
    matches.push({
      path: toWorkspaceRelativePath(workspaceRoot, absolutePath),
      line: Number(lineNumber),
      column: null,
      text,
    });
  }

  return matches;
}

export async function createBashTools(opts: BashToolsOptions, logger: Logger) {
  const defaultCwd = opts.cwd ?? ".";
  const defaultTimeoutMs = opts.timeoutMs ?? 30_000;
  const maxOutputBytes = opts.maxOutputBytes ?? 64 * 1024;
  const allowEnvKeys = opts.allowEnvKeys ?? [];

  const Bash = tool({
    description:
      "Run a bash command inside the configured workspace with timeout and output limits.",
    inputSchema: z.object({
      command: z.string().min(1),
      cwd: z.string().optional(),
      timeoutMs: z.number().int().min(1).max(300_000).optional(),
    }),
    execute: async ({ command, cwd, timeoutMs }) => {
      logger.debug('Running bash-agent shell tool "Bash"', {
        command,
        cwd,
        timeoutMs,
      });

      try {
        const commandCwd = resolveCommandCwd(
          opts.workspaceRoot,
          defaultCwd,
          cwd,
        );
        const result = await runCommand(command, {
          cwd: commandCwd,
          timeoutMs: timeoutMs ?? defaultTimeoutMs,
          maxOutputBytes,
          allowEnvKeys,
        });

        return {
          ok: result.exitCode === 0 && !result.timedOut,
          ...result,
          cwd: toWorkspaceRelativePath(opts.workspaceRoot, commandCwd),
        };
      } catch (error) {
        const nextError =
          error instanceof Error ? error : new Error(String(error));
        return {
          ok: false as const,
          error: nextError.message,
          exitCode: -1,
          stdout: "",
          stderr: "",
          timedOut: false,
          signalCode: null,
        };
      }
    },
  });

  const Grep = tool({
    description:
      "Search for text in files under the workspace. Prefers ripgrep and falls back to grep.",
    inputSchema: z.object({
      pattern: z.string().min(1),
      path: z.string().optional(),
      glob: z.string().optional(),
      caseSensitive: z.boolean().default(false),
      maxResults: z.number().int().min(1).max(500).default(100),
    }),
    execute: async ({ pattern, path, glob, caseSensitive, maxResults }) => {
      logger.debug('Running bash-agent shell tool "Grep"', {
        pattern,
        path,
        glob,
        caseSensitive,
        maxResults,
      });

      try {
        const commandCwd = resolveCommandCwd(
          opts.workspaceRoot,
          defaultCwd,
          path ?? ".",
        );
        const rgCheck = await runCommand("command -v rg >/dev/null 2>&1", {
          cwd: commandCwd,
          timeoutMs: defaultTimeoutMs,
          maxOutputBytes,
          allowEnvKeys,
        });

        if (rgCheck.exitCode === 0) {
          const rgParts = [
            "rg",
            "--json",
            "--line-number",
            "--color",
            "never",
            "--max-count",
            String(maxResults),
          ];
          if (!caseSensitive) {
            rgParts.push("-i");
          }
          if (glob) {
            rgParts.push("--glob", shellQuote(glob));
          }
          rgParts.push(shellQuote(pattern), ".");

          const rgResult = await runCommand(rgParts.join(" "), {
            cwd: commandCwd,
            timeoutMs: defaultTimeoutMs,
            maxOutputBytes,
            allowEnvKeys,
          });

          const matches = parseRipgrepJson(
            rgResult.stdout,
            opts.workspaceRoot,
          ).slice(0, maxResults);

          return {
            ok: rgResult.exitCode === 0 || rgResult.exitCode === 1,
            engine: "rg",
            matches,
            stderr: rgResult.stderr,
          };
        }

        const grepParts = ["grep", "-RIn"];
        if (!caseSensitive) {
          grepParts.push("-i");
        }
        if (glob) {
          grepParts.push("--include", shellQuote(glob));
        }
        grepParts.push(shellQuote(pattern), ".");

        const grepResult = await runCommand(grepParts.join(" "), {
          cwd: commandCwd,
          timeoutMs: defaultTimeoutMs,
          maxOutputBytes,
          allowEnvKeys,
        });

        return {
          ok: grepResult.exitCode === 0 || grepResult.exitCode === 1,
          engine: "grep",
          matches: parseGrepLines(grepResult.stdout, opts.workspaceRoot).slice(
            0,
            maxResults,
          ),
          stderr: grepResult.stderr,
        };
      } catch (error) {
        const nextError =
          error instanceof Error ? error : new Error(String(error));
        return {
          ok: false as const,
          error: nextError.message,
          matches: [],
        };
      }
    },
  });

  return {
    Bash,
    Grep,
  };
}
