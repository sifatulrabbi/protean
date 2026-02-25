import {
  mkdir,
  readdir,
  readFile,
  rename,
  rm,
  stat,
  writeFile,
} from "fs/promises";
import { resolve, relative, sep, normalize } from "path";
import { type Logger } from "@protean/logger";

import { type FS } from "./interfaces";
import { resolveWithinRoot } from "./path-utils";

export async function createLocalFs(
  rootPath: string,
  logger?: Logger,
): Promise<FS> {
  const root = resolve(rootPath);
  let writeQueue: Promise<void> = Promise.resolve();

  logger?.info("Initializing local workspace FS", { rootPath: root });

  await mkdir(root, { recursive: true });

  logger?.info("Initialized project at:", { rootPath: root });

  const rootStat = await stat(root);
  if (!rootStat.isDirectory()) {
    throw new Error(
      `Root path ${rootPath} is a file but requires a directory.`,
    );
  }

  async function withWriteLock<T>(task: () => Promise<T>): Promise<T> {
    const previous = writeQueue;
    let release: () => void = () => {};

    writeQueue = new Promise<void>((resolve) => {
      release = resolve;
    });

    await previous.catch(() => undefined);

    try {
      return await task();
    } finally {
      release();
    }
  }

  function resolveWritablePath(filePath: string): {
    resolvedPath: string;
    parentPath: string;
  } {
    const resolvedPath = resolveWithinRoot(root, filePath);
    const parentPath = normalize(resolve(resolvedPath, ".."));
    const rel = relative(root, parentPath);
    const escapesRoot = rel.startsWith("..") || rel.includes(`..${sep}`);
    if (escapesRoot) {
      throw new Error(`Path "${filePath}" escapes workspace root.`);
    }

    return { resolvedPath, parentPath };
  }

  return {
    stat: async (filePath) => {
      logger?.debug("FS.stat", { filePath });
      const resolvedPath = resolveWithinRoot(root, filePath);
      const fileStat = await stat(resolvedPath);
      return {
        isDirectory: fileStat.isDirectory(),
        size: fileStat.size,
        modified: fileStat.mtime.toISOString(),
        created: fileStat.birthtime.toISOString(),
      };
    },

    readdir: async (dirPath) => {
      logger?.debug("FS.readdir", { dirPath });
      const resolvedPath = resolveWithinRoot(root, dirPath);
      const dirEntries = await readdir(resolvedPath, { withFileTypes: true });

      return dirEntries.map((entry) => ({
        name: entry.name,
        isDirectory: entry.isDirectory(),
      }));
    },

    readFile: async (filePath) => {
      logger?.debug("FS.readFile", { filePath });
      const resolvedPath = resolveWithinRoot(root, filePath);
      return readFile(resolvedPath, "utf8");
    },

    readFileBuffer: async (filePath) => {
      logger?.debug("FS.readFileBuffer", { filePath });
      const resolvedPath = resolveWithinRoot(root, filePath);
      return readFile(resolvedPath);
    },

    mkdir: async (dirPath) => {
      logger?.debug("FS.mkdir", { dirPath });
      await withWriteLock(async () => {
        const resolvedPath = resolveWithinRoot(root, dirPath);
        await mkdir(resolvedPath, { recursive: true });
      });
    },

    writeFile: async (filePath, content) => {
      logger?.debug("FS.writeFile", { filePath, bytes: content.length });
      await withWriteLock(async () => {
        const { resolvedPath, parentPath } = resolveWritablePath(filePath);
        await mkdir(parentPath, { recursive: true });
        await writeFile(resolvedPath, content, "utf8");
      });
    },

    writeFileBuffer: async (filePath, content) => {
      logger?.debug("FS.writeFileBuffer", { filePath, bytes: content.length });
      await withWriteLock(async () => {
        const { resolvedPath, parentPath } = resolveWritablePath(filePath);
        await mkdir(parentPath, { recursive: true });
        await writeFile(resolvedPath, content);
      });
    },

    resolvePath: (filePath) => {
      return resolveWithinRoot(root, filePath);
    },

    remove: async (fullPath) => {
      logger?.debug("FS.remove", { fullPath });
      await withWriteLock(async () => {
        const resolvedPath = resolveWithinRoot(root, fullPath);
        await rm(resolvedPath, { recursive: true, force: true });
      });
    },

    move: async (sourcePath, destinationPath) => {
      logger?.debug("FS.move", { sourcePath, destinationPath });
      await withWriteLock(async () => {
        const sourceResolvedPath = resolveWithinRoot(root, sourcePath);
        const { resolvedPath: destinationResolvedPath, parentPath } =
          resolveWritablePath(destinationPath);
        await mkdir(parentPath, { recursive: true });
        await rename(sourceResolvedPath, destinationResolvedPath);
      });
    },
  };
}
