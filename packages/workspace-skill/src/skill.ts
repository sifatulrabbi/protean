import { tool } from "ai";
import { z } from "zod";
import { Skill } from "@protean/skill";
import { type Logger } from "@protean/logger";
import { tryCatch } from "@protean/utils";
import { type FS } from "@protean/vfs";

import { description, instructions } from "./instructions";

export interface WorkspaceSkillDeps {
  fsClient: FS;
  logger: Logger;
}

export class WorkspaceSkill extends Skill<WorkspaceSkillDeps> {
  constructor(dependencies: WorkspaceSkillDeps) {
    super(
      {
        id: "workspace-skill",
        description: description,
        instructions: instructions,
        dependencies: dependencies,
      },
      dependencies.logger,
    );
  }

  get tools() {
    return {
      Stat: this.Stat,
      ListDir: this.ListDir,
      ReadFile: this.ReadFile,
      Mkdir: this.Mkdir,
      WriteFile: this.WriteFile,
      Move: this.Move,
      Remove: this.Remove,
    };
  }

  Stat = tool({
    description: "Get metadata (size, type, timestamps) for files/directories.",
    inputSchema: z.object({
      fullPath: z
        .string()
        .describe("Path to the file or directory to inspect."),
    }),
    execute: async ({ fullPath }) => {
      this.dependencies.logger.debug('Running workspace operation "Stat"', {
        path: fullPath,
      });
      const { result, error } = await tryCatch(async () => {
        const stat = await this.dependencies.fsClient.stat(fullPath);
        if (stat.isDirectory) {
          return { ...stat, totalLines: null as number | null };
        }

        const content = await this.dependencies.fsClient.readFile(fullPath);
        return {
          ...stat,
          totalLines: content.split("\n").length,
        };
      });
      if (error) {
        this.logger.error('Workspace operation "Stat" failed', {
          path: fullPath,
          error,
        });
        return {
          error: `Workspace operation "Stat" failed for path "${fullPath}": ${error.message || "Unknown filesystem error"}`,
          fullPath,
        };
      }

      return { error: null, fullPath, ...result };
    },
  });

  ListDir = tool({
    description: "List directory entries.",
    inputSchema: z.object({
      fullPath: z.string().describe("Path to the directory to read."),
    }),
    execute: async ({ fullPath }) => {
      this.dependencies.logger.debug('Running workspace operation "ListDir"', {
        path: fullPath,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.fsClient.readdir(fullPath),
      );
      if (error) {
        this.logger.error('Workspace operation "ListDir" failed', {
          path: fullPath,
          error,
        });
        return {
          error: `Workspace operation "ListDir" failed for path "${fullPath}": ${error.message || "Unknown filesystem error"}`,
          fullPath,
        };
      }

      return { error: null, fullPath, entries: result };
    },
  });

  ReadFile = tool({
    description: "Read file contents.",
    inputSchema: z.object({
      fullPath: z.string().describe("Path to the file to read."),
    }),
    execute: async ({ fullPath }) => {
      this.dependencies.logger.debug('Running workspace operation "ReadFile"', {
        path: fullPath,
      });
      const { result, error } = await tryCatch(() =>
        this.dependencies.fsClient.readFile(fullPath),
      );
      if (error) {
        this.logger.error('Workspace operation "ReadFile" failed', {
          path: fullPath,
          error,
        });
        return {
          error: `Workspace operation "ReadFile" failed for path "${fullPath}": ${error.message || "Unknown filesystem error"}`,
          fullPath,
        };
      }

      return { error: null, fullPath, content: result };
    },
  });

  Mkdir = tool({
    description: "Create directories.",
    inputSchema: z.object({
      fullPath: z.string().describe("Path of the directory to create."),
    }),
    execute: async ({ fullPath }) => {
      this.dependencies.logger.debug('Running workspace operation "Mkdir"', {
        path: fullPath,
      });
      const { error } = await tryCatch(() =>
        this.dependencies.fsClient.mkdir(fullPath),
      );
      if (error) {
        this.logger.error('Workspace operation "Mkdir" failed', {
          path: fullPath,
          error,
        });
        return {
          error: `Workspace operation "Mkdir" failed for path "${fullPath}": ${error.message || "Unknown filesystem error"}`,
          path: fullPath,
          created: false,
        };
      }

      return { error: null, fullPath, created: true };
    },
  });

  WriteFile = tool({
    description:
      "Write content to files. Creates the file if it does not exist, overwrites if it does.",
    inputSchema: z.object({
      fullPath: z.string().describe("Path of the file to write."),
      content: z.string().describe("The text content to write to the file."),
    }),
    execute: async ({ fullPath, content }) => {
      this.dependencies.logger.debug(
        'Running workspace operation "WriteFile"',
        {
          path: fullPath,
          bytes: content.length,
        },
      );
      const { error } = await tryCatch(() =>
        this.dependencies.fsClient.writeFile(fullPath, content),
      );
      if (error) {
        this.logger.error('Workspace operation "WriteFile" failed', {
          path: fullPath,
          error,
        });
        return {
          error: `Workspace operation "WriteFile" failed for path "${fullPath}": ${error.message || "Unknown filesystem error"}`,
          fullPath,
          bytesWritten: 0,
        };
      }

      return {
        error: null,
        fullPath,
        bytesWritten: content.length,
      };
    },
  });

  Remove = tool({
    description: "Delete files or directories.",
    inputSchema: z.object({
      fullPath: z.string().describe("Path of the file or directory to remove"),
    }),
    execute: async ({ fullPath }) => {
      this.dependencies.logger.debug('Running workspace operation "Remove"', {
        path: fullPath,
      });
      const { error } = await tryCatch(() =>
        this.dependencies.fsClient.remove(fullPath),
      );
      if (error) {
        this.logger.error('Workspace operation "Remove" failed', {
          path: fullPath,
          error,
        });
        return {
          error: `Workspace operation "Remove" failed for path "${fullPath}": ${error.message || "Unknown filesystem error"}`,
          fullPath,
          removed: false,
        };
      }

      return { error: null, fullPath, removed: true };
    },
  });

  Move = tool({
    description: "Move/rename files or directories.",
    inputSchema: z.object({
      sourcePath: z
        .string()
        .describe("Current full path of the file or directory."),
      destinationPath: z
        .string()
        .describe("Destination full path for the file or directory."),
    }),
    execute: async ({ sourcePath, destinationPath }) => {
      this.dependencies.logger.debug('Running workspace operation "Move"', {
        sourcePath,
        destinationPath,
      });

      const { error } = await tryCatch(() =>
        this.dependencies.fsClient.move(sourcePath, destinationPath),
      );

      if (error) {
        this.logger.error('Workspace operation "Move" failed', {
          sourcePath,
          destinationPath,
          error,
        });
        return {
          error: `Workspace operation "Move" failed for source "${sourcePath}" to destination "${destinationPath}": ${error.message || "Unknown filesystem error"}`,
          sourcePath,
          destinationPath,
          moved: false,
        };
      }

      return {
        error: null,
        sourcePath,
        destinationPath,
        moved: true,
      };
    },
  });
}
