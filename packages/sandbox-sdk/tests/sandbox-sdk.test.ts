import { describe, expect, it } from "bun:test";

import { createProject, createSandboxClient, toScopedPath } from "../src";

describe("toScopedPath", () => {
  it("scopes relative paths into a project root", () => {
    expect(toScopedPath("/workspace/projects/protean", "docs/readme.md")).toBe(
      "/workspace/projects/protean/docs/readme.md",
    );
  });

  it("keeps already scoped paths intact", () => {
    expect(toScopedPath("/workspace/projects/protean", "/workspace/projects/protean/src/index.ts")).toBe(
      "/workspace/projects/protean/src/index.ts",
    );
  });
});

describe("createProject", () => {
  it("delegates scoped filesystem operations", async () => {
    const writes: string[] = [];
    const project = createProject({
      projectPath: "/workspace/projects/demo",
      sandboxClient: {
        isConnected: true,
        connErr: null,
        bash: {
          exec: async () => ({
            result: { stdout: "", stderr: "" },
            error: null,
          }),
        },
        fs: {
          readFile: async (fullPath) => ({ fullPath, content: "", range: { startLine: 0 }, error: null }),
          listDir: async (fullPath) => ({ fullPath, entries: [], error: null }),
          stat: async (fullPath) => ({ fullPath, entity: null, error: null }),
          writeFile: async (fullPath) => {
            writes.push(fullPath);
            return { fullPath, error: null };
          },
          create: async (fullPath) => ({
            fullPath,
            entity: {
              name: "notes.md",
              fullPath,
              type: "file",
              size: 0,
              modifiedAt: null,
            },
            error: null,
          }),
          copy: async (sourceFullPath, copyToFullPath) => ({ sourceFullPath, copyToFullPath, error: null }),
          move: async (sourceFullPath, newFullPath) => ({ sourceFullPath, newFullPath, error: null }),
          remove: async (fullPath) => ({ fullPath, error: null }),
        },
      },
    });

    await project.fs.writeFile("notes.md", "hello");

    expect(writes).toEqual(["/workspace/projects/demo/notes.md"]);
  });
});

describe("HttpSandboxClient", () => {
  it("loads project lists from the sandbox service", async () => {
    const client = createSandboxClient({
      baseUrl: "http://sandbox.local",
      fetch: async () =>
        new Response(
          JSON.stringify([
            {
              name: "protean",
              fullPath: "/workspace/projects/protean",
              source: "remote",
              mountId: null,
            },
          ]),
          { status: 200 },
        ),
    });

    const projects = await client.listProjects();

    expect(projects).toHaveLength(1);
    expect(projects[0]?.name).toBe("protean");
  });

  it("creates mounted projects through the sandbox service", async () => {
    const client = createSandboxClient({
      baseUrl: "http://sandbox.local",
      fetch: async () =>
        new Response(
          JSON.stringify({
            id: "mount_1",
            workspacePath: "/workspace/projects/protean",
            sourcePath: "/Users/demo/protean",
            projectName: "protean",
            createdAt: new Date().toISOString(),
          }),
          { status: 200 },
        ),
    });

    const mount = await client.createMount({
      sourcePath: "/Users/demo/protean",
      projectName: "protean",
    });

    expect(mount.projectName).toBe("protean");
  });
});
