import { join } from "node:path";
import { tool } from "ai";
import type { Logger } from "@protean/logger";
import { createLocalFs } from "@protean/vfs";
import { tryCatch } from "@protean/utils";
import { z } from "zod";

import {
  resolveWithinWorkspace,
  toWorkspaceRelativePath,
} from "./workspace-paths";

export interface FsToolsOptions {
  workspaceRoot: string;
  cwd?: string;
  maxReadBytes?: number;
  maxReadLines?: number;
}

function successResult<T extends Record<string, unknown>>(
  result: T,
): T & {
  ok: true;
} {
  return {
    ok: true,
    ...result,
  };
}

function failureResult<T extends Record<string, unknown>>(
  error: Error,
  result: T,
): T & { ok: false; error: string } {
  return {
    ok: false,
    error: error.message,
    ...result,
  };
}

function resolveToolPath(
  workspaceRoot: string,
  cwd: string,
  targetPath?: string,
): string {
  const resolvedCwd = resolveWithinWorkspace(workspaceRoot, cwd);
  return resolveWithinWorkspace(
    workspaceRoot,
    targetPath ? join(resolvedCwd, targetPath) : resolvedCwd,
  );
}

function countOccurrences(content: string, oldString: string): number {
  if (oldString.length === 0) {
    return 0;
  }

  let count = 0;
  let offset = 0;

  while (true) {
    const index = content.indexOf(oldString, offset);
    if (index === -1) {
      return count;
    }
    count += 1;
    offset = index + oldString.length;
  }
}

function withLineNumbers(lines: string[], startLine: number): string {
  return lines.map((line, index) => `${startLine + index}: ${line}`).join("\n");
}

export async function createFsTools(opts: FsToolsOptions, logger: Logger) {
  const fsClient = await createLocalFs(opts.workspaceRoot, logger);
  const defaultCwd = opts.cwd ?? ".";
  const maxReadBytes = opts.maxReadBytes ?? 64 * 1024;
  const maxReadLines = opts.maxReadLines ?? 2_000;

  const ListDir = tool({
    description:
      "List directory entries in the workspace. Can recurse to a bounded depth.",
    inputSchema: z.object({
      path: z.string().default("."),
      recursive: z.boolean().default(false),
      maxDepth: z.number().int().min(1).max(25).optional(),
    }),
    execute: async ({ path, recursive, maxDepth }) => {
      const basePath = path || ".";
      logger.debug('Running bash-agent fs tool "ListDir"', {
        path: basePath,
        recursive,
        maxDepth,
      });

      const resolvedBase = resolveToolPath(
        opts.workspaceRoot,
        defaultCwd,
        basePath,
      );
      const normalizedBase = toWorkspaceRelativePath(
        opts.workspaceRoot,
        resolvedBase,
      );
      const entries: Array<{
        path: string;
        name: string;
        isDirectory: boolean;
      }> = [];
      const limit = recursive ? (maxDepth ?? 8) : 1;

      const visit = async (
        currentPath: string,
        depth: number,
      ): Promise<void> => {
        const children = await fsClient.readdir(currentPath);
        for (const child of children) {
          const childPath =
            currentPath === "." ? child.name : `${currentPath}/${child.name}`;
          entries.push({
            path: childPath,
            name: child.name,
            isDirectory: child.isDirectory,
          });

          if (recursive && child.isDirectory && depth < limit) {
            await visit(childPath, depth + 1);
          }
        }
      };

      const { error } = await tryCatch(() => visit(normalizedBase, 1));
      if (error) {
        return failureResult(error, { path: basePath, entries: [] });
      }

      return successResult({ path: basePath, entries });
    },
  });

  const MoveEntity = tool({
    description: "Move or rename a file or directory inside the workspace.",
    inputSchema: z.object({
      sourcePath: z.string().min(1),
      destinationPath: z.string().min(1),
    }),
    execute: async ({ sourcePath, destinationPath }) => {
      logger.debug('Running bash-agent fs tool "MoveEntity"', {
        sourcePath,
        destinationPath,
      });

      const sourceAbs = resolveToolPath(
        opts.workspaceRoot,
        defaultCwd,
        sourcePath,
      );
      const destinationAbs = resolveToolPath(
        opts.workspaceRoot,
        defaultCwd,
        destinationPath,
      );
      const sourceRel = toWorkspaceRelativePath(opts.workspaceRoot, sourceAbs);
      const destinationRel = toWorkspaceRelativePath(
        opts.workspaceRoot,
        destinationAbs,
      );

      const { error } = await tryCatch(() =>
        fsClient.move(sourceRel, destinationRel),
      );
      if (error) {
        return failureResult(error, {
          sourcePath,
          destinationPath,
          moved: false,
        });
      }

      return successResult({
        sourcePath,
        destinationPath,
        moved: true,
      });
    },
  });

  const ReadFile = tool({
    description:
      "Read a text file from the workspace, optionally by line range and with line numbers.",
    inputSchema: z.object({
      path: z.string().min(1),
      startLine: z.number().int().min(1).optional(),
      endLine: z.number().int().min(1).optional(),
      withLineNumbers: z.boolean().default(false),
    }),
    execute: async ({
      path,
      startLine,
      endLine,
      withLineNumbers: includeLineNumbers,
    }) => {
      logger.debug('Running bash-agent fs tool "ReadFile"', {
        path,
        startLine,
        endLine,
      });

      const absolutePath = resolveToolPath(
        opts.workspaceRoot,
        defaultCwd,
        path,
      );
      const filePath = toWorkspaceRelativePath(
        opts.workspaceRoot,
        absolutePath,
      );
      const { result: content, error } = await tryCatch(() =>
        fsClient.readFile(filePath),
      );
      if (error) {
        return failureResult(error, { path, content: "" });
      }

      if (Buffer.byteLength(content, "utf8") > maxReadBytes) {
        return {
          ok: false as const,
          error: `File exceeds max readable size of ${maxReadBytes} bytes.`,
          path,
          content: "",
        };
      }

      const allLines = content.split("\n");
      if (allLines.length > maxReadLines) {
        return {
          ok: false as const,
          error: `File exceeds max readable line count of ${maxReadLines}.`,
          path,
          content: "",
        };
      }

      const sliceStart = Math.max(1, startLine ?? 1);
      const sliceEnd = Math.max(sliceStart, endLine ?? allLines.length);
      const selectedLines = allLines.slice(sliceStart - 1, sliceEnd);
      const rendered = includeLineNumbers
        ? withLineNumbers(selectedLines, sliceStart)
        : selectedLines.join("\n");

      return successResult({
        path,
        startLine: sliceStart,
        endLine: sliceStart + Math.max(selectedLines.length - 1, 0),
        content: rendered,
        totalLines: allLines.length,
      });
    },
  });

  const EntityStat = tool({
    description:
      "Get metadata for a file or directory, including line count for text files.",
    inputSchema: z.object({
      path: z.string().default("."),
    }),
    execute: async ({ path }) => {
      logger.debug('Running bash-agent fs tool "EntityStat"', { path });

      const absolutePath = resolveToolPath(
        opts.workspaceRoot,
        defaultCwd,
        path,
      );
      const relativePath = toWorkspaceRelativePath(
        opts.workspaceRoot,
        absolutePath,
      );
      const { result: stats, error } = await tryCatch(() =>
        fsClient.stat(relativePath),
      );
      if (error) {
        return failureResult(error, { path });
      }

      let totalLines: number | null = null;
      if (!stats.isDirectory) {
        const readResult = await tryCatch(() =>
          fsClient.readFile(relativePath),
        );
        if (!readResult.error) {
          totalLines = readResult.result.split("\n").length;
        }
      }

      return successResult({
        path,
        ...stats,
        totalLines,
      });
    },
  });

  const CreateEntity = tool({
    description:
      "Create a file or directory in the workspace. File overwrites require explicit confirmation.",
    inputSchema: z.object({
      path: z.string().min(1),
      kind: z.enum(["file", "directory"]),
      content: z.string().optional(),
      overwrite: z.boolean().default(false),
    }),
    execute: async ({ path, kind, content, overwrite }) => {
      logger.debug('Running bash-agent fs tool "CreateEntity"', {
        path,
        kind,
        overwrite,
      });

      const absolutePath = resolveToolPath(
        opts.workspaceRoot,
        defaultCwd,
        path,
      );
      const relativePath = toWorkspaceRelativePath(
        opts.workspaceRoot,
        absolutePath,
      );
      const existing = await tryCatch(() => fsClient.stat(relativePath));

      if (!existing.error && kind === "file" && !overwrite) {
        return {
          ok: false as const,
          error: `File "${path}" already exists. Set overwrite=true to replace it.`,
          path,
          created: false,
        };
      }

      if (kind === "directory") {
        const { error } = await tryCatch(() => fsClient.mkdir(relativePath));
        if (error) {
          return failureResult(error, { path, created: false });
        }

        return successResult({ path, created: true, kind });
      }

      const { error } = await tryCatch(() =>
        fsClient.writeFile(relativePath, content ?? ""),
      );
      if (error) {
        return failureResult(error, { path, created: false });
      }

      return successResult({
        path,
        created: true,
        kind,
        bytesWritten: Buffer.byteLength(content ?? "", "utf8"),
      });
    },
  });

  const EditFile = tool({
    description:
      "Edit a file by replacing an exact string with a new string, with optional occurrence checks.",
    inputSchema: z.object({
      path: z.string().min(1),
      oldString: z.string(),
      newString: z.string(),
      replaceAll: z.boolean().default(false),
      expectedOccurrences: z.number().int().min(0).optional(),
    }),
    execute: async ({
      path,
      oldString,
      newString,
      replaceAll,
      expectedOccurrences,
    }) => {
      logger.debug('Running bash-agent fs tool "EditFile"', {
        path,
        replaceAll,
        expectedOccurrences,
      });

      if (!oldString) {
        return {
          ok: false as const,
          error: "oldString must not be empty.",
          path,
        };
      }

      const absolutePath = resolveToolPath(
        opts.workspaceRoot,
        defaultCwd,
        path,
      );
      const relativePath = toWorkspaceRelativePath(
        opts.workspaceRoot,
        absolutePath,
      );
      const { result: content, error } = await tryCatch(() =>
        fsClient.readFile(relativePath),
      );
      if (error) {
        return failureResult(error, { path, replaced: false });
      }

      const matches = countOccurrences(content, oldString);
      const replacements = replaceAll ? matches : Math.min(matches, 1);

      if (replacements === 0) {
        return {
          ok: false as const,
          error: `oldString not found in "${path}".`,
          path,
          replaced: false,
        };
      }

      if (
        typeof expectedOccurrences === "number" &&
        expectedOccurrences !== replacements
      ) {
        return {
          ok: false as const,
          error: `Expected ${expectedOccurrences} replacements but found ${replacements}.`,
          path,
          replaced: false,
        };
      }

      const nextContent = replaceAll
        ? content.split(oldString).join(newString)
        : content.replace(oldString, newString);
      const writeResult = await tryCatch(() =>
        fsClient.writeFile(relativePath, nextContent),
      );
      if (writeResult.error) {
        return failureResult(writeResult.error, { path, replaced: false });
      }

      return successResult({
        path,
        replaced: true,
        replacements,
      });
    },
  });

  const RemoveEntity = tool({
    description:
      "Remove a file or directory. Directories require recursive=true.",
    inputSchema: z.object({
      path: z.string().min(1),
      recursive: z.boolean().default(false),
    }),
    execute: async ({ path, recursive }) => {
      logger.debug('Running bash-agent fs tool "RemoveEntity"', {
        path,
        recursive,
      });

      const absolutePath = resolveToolPath(
        opts.workspaceRoot,
        defaultCwd,
        path,
      );
      const relativePath = toWorkspaceRelativePath(
        opts.workspaceRoot,
        absolutePath,
      );
      const statResult = await tryCatch(() => fsClient.stat(relativePath));
      if (statResult.error) {
        return failureResult(statResult.error, { path, removed: false });
      }

      if (statResult.result.isDirectory && !recursive) {
        return {
          ok: false as const,
          error: `Directory "${path}" requires recursive=true to remove.`,
          path,
          removed: false,
        };
      }

      const { error } = await tryCatch(() => fsClient.remove(relativePath));
      if (error) {
        return failureResult(error, { path, removed: false });
      }

      return successResult({ path, removed: true });
    },
  });

  const Glob = tool({
    description:
      "Match files in the workspace with a glob pattern, returning workspace-relative paths.",
    inputSchema: z.object({
      pattern: z.string().min(1),
      cwd: z.string().optional(),
      includeDirectories: z.boolean().default(false),
      maxResults: z.number().int().min(1).max(1_000).default(200),
    }),
    execute: async ({ pattern, cwd, includeDirectories, maxResults }) => {
      logger.debug('Running bash-agent fs tool "Glob"', {
        pattern,
        cwd,
        includeDirectories,
        maxResults,
      });

      const absoluteCwd = resolveToolPath(
        opts.workspaceRoot,
        defaultCwd,
        cwd ?? ".",
      );
      const cwdRelative = toWorkspaceRelativePath(
        opts.workspaceRoot,
        absoluteCwd,
      );
      const results: string[] = [];
      const glob = new Bun.Glob(pattern);

      try {
        for await (const match of glob.scan({
          cwd: absoluteCwd,
          absolute: false,
          onlyFiles: !includeDirectories,
        })) {
          const absoluteMatch = resolveToolPath(
            opts.workspaceRoot,
            cwdRelative,
            match,
          );
          results.push(
            toWorkspaceRelativePath(opts.workspaceRoot, absoluteMatch),
          );
          if (results.length >= maxResults) {
            break;
          }
        }
      } catch (error) {
        const nextError =
          error instanceof Error ? error : new Error(String(error));
        return failureResult(nextError, { pattern, matches: [] });
      }

      return successResult({
        pattern,
        matches: [...new Set(results)],
      });
    },
  });

  return {
    ListDir,
    MoveEntity,
    ReadFile,
    EntityStat,
    CreateEntity,
    EditFile,
    RemoveEntity,
    Glob,
  };
}
