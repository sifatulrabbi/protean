import { beforeEach, describe, expect, mock, test } from "bun:test";

const ensureCalls: Array<{
  baseUrl: string;
  serviceToken: string;
  sessionId: string;
}> = [];

await mock.module("@protean/sandbox-client", () => ({
  ensureSandboxSession: async (
    config: {
      baseUrl: string;
      serviceToken: string;
    },
    input: { sessionId: string },
  ) => {
    ensureCalls.push({
      baseUrl: config.baseUrl,
      serviceToken: config.serviceToken,
      sessionId: input.sessionId,
    });

    return {
      sessionId: input.sessionId,
      workspaceMountPath: "/workspace",
      workspaceFullPath: "/tmp/workspace",
      fs: {
        stat: async () => ({
          isDirectory: true,
          size: 0,
          modified: new Date(0).toISOString(),
          created: new Date(0).toISOString(),
        }),
        readdir: async () => [],
        readFile: async () => "",
        readFileBuffer: async () => Buffer.from(""),
        mkdir: async () => {},
        writeFile: async () => {},
        writeFileBuffer: async () => {},
        move: async () => {},
        remove: async () => {},
        resolvePath: (p: string) => p,
      },
      getSession: async () => ({
        sessionId: input.sessionId,
        workspaceMountPath: "/workspace",
        workspaceFullPath: "/tmp/workspace",
        containerName: "sandbox",
        image: "sandbox",
        state: "running",
        createdAt: new Date(0).toISOString(),
        exists: true,
        workspaceReady: true,
        containerPresent: true,
        containerRunning: true,
      }),
      exec: async () => ({
        exitCode: 0,
        stdout: "",
        stderr: "",
        timedOut: false,
        signalCode: null,
      }),
      deleteSession: async () => {},
    };
  },
}));

const { getWorkspaceSandbox } = await import("@/lib/server/workspace-fs");

describe("workspace-fs sandbox session bootstrap", () => {
  beforeEach(() => {
    ensureCalls.length = 0;

    process.env.SANDBOX_BASE_URL = "http://localhost:8091";
    process.env.SANDBOX_SERVICE_TOKEN = "token";
  });

  test("uses user id as deterministic sandbox session id", async () => {
    const workspace = await getWorkspaceSandbox("alice@example.com");

    expect(workspace.sessionId).toBe("alice@example.com");
    expect(ensureCalls[0]).toEqual({
      baseUrl: "http://localhost:8091",
      serviceToken: "token",
      sessionId: "alice@example.com",
    });
  });

  test("reuses in-process cached sandbox bootstrap promise", async () => {
    const [first, second] = await Promise.all([
      getWorkspaceSandbox("bob@example.com"),
      getWorkspaceSandbox("bob@example.com"),
    ]);

    expect(first.sessionId).toBe("bob@example.com");
    expect(second.sessionId).toBe("bob@example.com");
    expect(
      ensureCalls.filter((call) => call.sessionId === "bob@example.com"),
    ).toHaveLength(1);
  });
});
