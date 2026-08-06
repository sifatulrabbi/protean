import {
  cp,
  lstat,
  mkdir,
  readdir,
  readFile,
  rename,
  rm,
  stat,
  symlink,
  writeFile,
} from "node:fs/promises";
import path from "node:path";
import type { Entity, FileRange, SandboxProject } from "@protean/contracts";

import type { SandboxServerConfig } from "./config";

function toVirtualPath(config: SandboxServerConfig, hostPath: string) {
  const relative = path.relative(config.workspaceRoot, hostPath);
  return path.posix.join(
    config.virtualWorkspaceRoot,
    relative.split(path.sep).join(path.posix.sep),
  );
}

export async function ensureWorkspace(config: SandboxServerConfig) {
  await mkdir(config.projectsRoot, { recursive: true });
  await mkdir(config.mountsRoot, { recursive: true });
}

export function toHostPath(config: SandboxServerConfig, virtualPath: string) {
  const normalizedVirtualRoot = config.virtualWorkspaceRoot.replace(/\/+$/, "");
  const normalizedVirtualPath = virtualPath.startsWith(normalizedVirtualRoot)
    ? virtualPath
    : path.posix.join(normalizedVirtualRoot, virtualPath);
  const relative = normalizedVirtualPath
    .slice(normalizedVirtualRoot.length)
    .replace(/^\/+/, "");
  const hostPath = path.resolve(config.workspaceRoot, relative);
  const resolvedWorkspaceRoot = path.resolve(config.workspaceRoot);

  if (
    hostPath !== resolvedWorkspaceRoot &&
    !hostPath.startsWith(`${resolvedWorkspaceRoot}${path.sep}`)
  ) {
    throw new Error(`Path escapes sandbox workspace: ${virtualPath}`);
  }

  return hostPath;
}

export async function toEntity(
  config: SandboxServerConfig,
  hostPath: string,
): Promise<Entity> {
  const fileStats = await lstat(hostPath);
  const type: Entity["type"] = fileStats.isDirectory()
    ? "directory"
    : fileStats.isSymbolicLink()
      ? "symlink"
      : "file";

  return {
    name: path.basename(hostPath),
    fullPath: toVirtualPath(config, hostPath),
    type,
    size: fileStats.size,
    modifiedAt: fileStats.mtime ? fileStats.mtime.toISOString() : null,
  };
}

export async function listProjects(
  config: SandboxServerConfig,
): Promise<SandboxProject[]> {
  await ensureWorkspace(config);

  const entries = await readdir(config.projectsRoot, { withFileTypes: true });
  const projects = await Promise.all(
    entries
      .filter((entry) => entry.isDirectory() || entry.isSymbolicLink())
      .map(async (entry) => {
        const fullPath = path.join(config.projectsRoot, entry.name);
        const source = entry.isSymbolicLink() ? "mount" : "remote";

        return {
          name: entry.name,
          fullPath: toVirtualPath(config, fullPath),
          source,
          mountId: source === "mount" ? entry.name : null,
        } satisfies SandboxProject;
      }),
  );

  return projects.sort((left, right) => left.name.localeCompare(right.name));
}

export async function readVirtualFile(
  config: SandboxServerConfig,
  virtualPath: string,
  range?: FileRange,
) {
  const hostPath = toHostPath(config, virtualPath);
  const content = await readFile(hostPath, "utf8");
  const lines = content.split("\n");
  const startLine = range?.startLine ?? 0;
  const endLine = range?.endLine ?? lines.length;

  return {
    fullPath: path.posix.normalize(virtualPath),
    content: lines.slice(startLine, endLine).join("\n"),
    range: {
      startLine,
      ...(range?.endLine ? { endLine } : {}),
    },
  };
}

export async function listVirtualDir(
  config: SandboxServerConfig,
  virtualPath: string,
) {
  const hostPath = toHostPath(config, virtualPath);
  const entries = await readdir(hostPath);
  const detailedEntries = await Promise.all(
    entries.map((entry) => toEntity(config, path.join(hostPath, entry))),
  );

  return {
    fullPath: path.posix.normalize(virtualPath),
    entries: detailedEntries.sort((left, right) =>
      left.name.localeCompare(right.name),
    ),
  };
}

export async function statVirtualPath(
  config: SandboxServerConfig,
  virtualPath: string,
) {
  const hostPath = toHostPath(config, virtualPath);

  try {
    await stat(hostPath);
    return {
      fullPath: path.posix.normalize(virtualPath),
      entity: await toEntity(config, hostPath),
    };
  } catch {
    return {
      fullPath: path.posix.normalize(virtualPath),
      entity: null,
    };
  }
}

export async function writeVirtualFile(
  config: SandboxServerConfig,
  virtualPath: string,
  content: string,
) {
  const hostPath = toHostPath(config, virtualPath);

  await mkdir(path.dirname(hostPath), { recursive: true });
  await writeFile(hostPath, content);

  return { fullPath: path.posix.normalize(virtualPath) };
}

export async function createVirtualEntity(
  config: SandboxServerConfig,
  virtualPath: string,
  entityType: "file" | "directory",
  recursive = false,
) {
  const hostPath = toHostPath(config, virtualPath);

  if (entityType === "directory") {
    await mkdir(hostPath, { recursive });
  } else {
    await mkdir(path.dirname(hostPath), { recursive: true });
    await writeFile(hostPath, "");
  }

  return {
    fullPath: path.posix.normalize(virtualPath),
    entity: await toEntity(config, hostPath),
  };
}

export async function copyVirtualPath(
  config: SandboxServerConfig,
  sourceVirtualPath: string,
  targetVirtualPath: string,
) {
  const sourceHostPath = toHostPath(config, sourceVirtualPath);
  const targetHostPath = toHostPath(config, targetVirtualPath);

  await cp(sourceHostPath, targetHostPath, { recursive: true });

  return {
    sourceFullPath: path.posix.normalize(sourceVirtualPath),
    copyToFullPath: path.posix.normalize(targetVirtualPath),
  };
}

export async function moveVirtualPath(
  config: SandboxServerConfig,
  sourceVirtualPath: string,
  targetVirtualPath: string,
) {
  const sourceHostPath = toHostPath(config, sourceVirtualPath);
  const targetHostPath = toHostPath(config, targetVirtualPath);

  await mkdir(path.dirname(targetHostPath), { recursive: true });
  await rename(sourceHostPath, targetHostPath);

  return {
    sourceFullPath: path.posix.normalize(sourceVirtualPath),
    newFullPath: path.posix.normalize(targetVirtualPath),
  };
}

export async function removeVirtualPath(
  config: SandboxServerConfig,
  virtualPath: string,
  recursive = false,
) {
  const hostPath = toHostPath(config, virtualPath);

  await rm(hostPath, { recursive, force: true });

  return {
    fullPath: path.posix.normalize(virtualPath),
  };
}

export async function createMountedProject(
  config: SandboxServerConfig,
  payload: {
    sourcePath: string;
    projectName: string;
  },
) {
  await ensureWorkspace(config);

  if (
    !/^[a-zA-Z0-9._-]+$/.test(payload.projectName) ||
    payload.projectName === "." ||
    payload.projectName === ".."
  ) {
    throw new Error("Project name must be a safe path segment.");
  }

  let candidateName = payload.projectName;
  let counter = 1;

  while (true) {
    const candidatePath = path.join(config.projectsRoot, candidateName);

    try {
      await lstat(candidatePath);
      counter += 1;
      candidateName = `${payload.projectName}-${counter}`;
    } catch {
      await symlink(payload.sourcePath, candidatePath, "dir");
      return {
        id: candidateName,
        workspacePath: toVirtualPath(config, candidatePath),
        sourcePath: payload.sourcePath,
        projectName: candidateName,
        createdAt: new Date().toISOString(),
      };
    }
  }
}

export async function executeScopedCommand(
  config: SandboxServerConfig,
  payload: { cmd: string; scope?: string },
) {
  const workingDirectory = payload.scope
    ? toHostPath(config, payload.scope)
    : config.workspaceRoot;

  const proc = Bun.spawn(["/bin/sh", "-lc", payload.cmd], {
    cwd: workingDirectory,
    stdout: "pipe",
    stderr: "pipe",
  });
  const [stdout, stderr] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
  ]);
  const exitCode = await proc.exited;

  return {
    result: {
      stdout,
      stderr,
    },
    error: exitCode === 0 ? null : `Command exited with code ${exitCode}`,
  };
}
