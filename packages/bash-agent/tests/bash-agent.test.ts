import { afterEach, describe, expect, it } from "bun:test";
import { mkdir, rm } from "node:fs/promises";
import path from "node:path";

import { createFileAgentMemory } from "@protean/agent-memory";
import type { HttpSandboxClient } from "@protean/sandbox-sdk";
import { runAgentTurn } from "../src";

const testRoot = path.join(process.cwd(), "tmp", "bash-agent-tests");

afterEach(async () => {
  await rm(testRoot, { recursive: true, force: true });
});

describe("runAgentTurn", () => {
  it("stores a full fallback turn without a provider key", async () => {
    await rm(testRoot, { recursive: true, force: true });
    await mkdir(path.join(testRoot, "users", "demo", "projects", "demo"), {
      recursive: true,
    });

    const memory = await createFileAgentMemory({ baseDir: testRoot });
    const sandboxClient: HttpSandboxClient = {
      isConnected: true,
      connErr: null,
      async listProjects() {
        return [];
      },
      async getHealth() {
        return {
          ok: true,
          workspaceRoot: "/workspace",
          projectsRoot: "/workspace/projects",
          mountsRoot: "/workspace/mounts",
        };
      },
      startHeartbeat() {},
      stopHeartbeat() {},
      bash: {
        exec: async () => ({
          result: { stdout: "", stderr: "" },
          error: null,
        }),
      },
      fs: {
        readFile: async (fullPath: string) => ({
          fullPath,
          content: "",
          range: { startLine: 0 },
          error: null,
        }),
        listDir: async () => ({
          fullPath: "/workspace/projects/demo",
          entries: [
            {
              name: "README.md",
              fullPath: "/workspace/projects/demo/README.md",
              type: "file",
              size: 100,
              modifiedAt: new Date().toISOString(),
            },
          ],
          error: null,
        }),
        stat: async (fullPath: string) => ({
          fullPath,
          entity: null,
          error: null,
        }),
        writeFile: async (fullPath: string) => ({ fullPath, error: null }),
        create: async (fullPath: string) => ({
          fullPath,
          entity: {
            name: "a",
            fullPath,
            type: "file",
            size: 0,
            modifiedAt: null,
          },
          error: null,
        }),
        copy: async (sourceFullPath: string, copyToFullPath: string) => ({
          sourceFullPath,
          copyToFullPath,
          error: null,
        }),
        move: async (sourceFullPath: string, newFullPath: string) => ({
          sourceFullPath,
          newFullPath,
          error: null,
        }),
        remove: async (fullPath: string) => ({ fullPath, error: null }),
      },
    } as HttpSandboxClient;
    const result = await runAgentTurn({
      forceFallback: true,
      memory,
      sandboxClient,
      message: "Summarize the repo.",
      userId: "demo",
      projectName: "demo",
      modelId: "gpt-4o-mini",
      inferenceProvider: "openrouter",
    });

    expect(result.reply).toContain("deterministic local fallback");
    expect(result.thread.messages).toHaveLength(2);
  });
});
