import { afterEach, describe, expect, it } from "bun:test";
import { mkdir, rm, writeFile } from "node:fs/promises";
import path from "node:path";

import type { SandboxServerConfig } from "../src/config";
import {
  createMountedProject,
  listProjects,
  readVirtualFile,
  toHostPath,
} from "../src/filesystem";

const workspaceRoot = path.join(process.cwd(), "tmp", "sandbox-server-tests");

const config: SandboxServerConfig = {
  host: "127.0.0.1",
  port: 8787,
  workspaceRoot,
  virtualWorkspaceRoot: "/workspace",
  projectsRoot: path.join(workspaceRoot, "projects"),
  mountsRoot: path.join(workspaceRoot, "mounts"),
  serviceToken: undefined,
};

afterEach(async () => {
  await rm(workspaceRoot, { recursive: true, force: true });
});

describe("toHostPath", () => {
  it("maps virtual workspace paths to the host workspace", () => {
    expect(toHostPath(config, "/workspace/projects/demo")).toBe(
      path.join(workspaceRoot, "projects", "demo"),
    );
  });

  it("rejects paths that escape into a sibling with the same prefix", () => {
    expect(() =>
      toHostPath(config, "/workspace/../sandbox-server-tests-escape/file.txt"),
    ).toThrow("Path escapes sandbox workspace");
  });
});

describe("createMountedProject", () => {
  it("rejects project names that escape the projects directory", async () => {
    expect(
      createMountedProject(config, {
        sourcePath: workspaceRoot,
        projectName: "../escape",
      }),
    ).rejects.toThrow("Project name must be a safe path segment");
  });
});

describe("readVirtualFile", () => {
  it("reads line ranges from virtual files", async () => {
    await mkdir(path.join(config.projectsRoot, "demo"), { recursive: true });
    await writeFile(
      path.join(config.projectsRoot, "demo", "notes.md"),
      "one\ntwo\nthree",
    );

    const result = await readVirtualFile(
      config,
      "/workspace/projects/demo/notes.md",
      {
        startLine: 1,
        endLine: 3,
      },
    );

    expect(result.content).toBe("two\nthree");
  });
});

describe("listProjects", () => {
  it("returns remote projects", async () => {
    await mkdir(path.join(config.projectsRoot, "demo"), { recursive: true });

    const projects = await listProjects(config);

    expect(projects[0]?.fullPath).toBe("/workspace/projects/demo");
  });
});
