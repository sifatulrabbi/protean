import { afterEach, beforeEach, describe, expect, mock, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import type { LanguageModel, Tool, UIMessage } from "ai";
import {
  createFsMemory,
  deriveActiveHistory,
  type AgentMemory,
  type ThreadRecord,
} from "@protean/agent-memory";
import { noopLogger } from "@protean/logger";
import { findModel, getDefaultModelSelection } from "@protean/model-catalog";
import { createLocalFs } from "@protean/vfs";

let generateCalls: Array<{ messages: unknown[] }> = [];
let createAgentCalls: Array<{
  instructions?: string;
  tools?: Record<string, Tool>;
}> = [];
const sandboxClientState = {
  execCalls: [] as Array<{ command: string; cwd?: string; timeoutMs?: number }>,
  ensuredProjects: [] as string[],
  readCalls: [] as string[],
  workspaceMountPath: "/workspace",
  skillDirs: [] as Array<{ name: string; isDirectory: boolean }>,
  skillFiles: {} as Record<string, string>,
};

await mock.module("./base-agent", () => ({
  createAgent: (args: {
    instructions?: string;
    tools?: Record<string, Tool>;
  }) => {
    createAgentCalls.push(args);

    return {
      agent: { id: "mock-bash-agent" },
      generate: async (args: { messages: unknown[] }) => {
        generateCalls.push(args);
        return {
          text: "assistant reply",
          usage: {
            inputTokens: 12,
            outputTokens: 4,
          },
        };
      },
      stream: async () => {
        throw new Error("stream not implemented in tests");
      },
      asTool: () => ({
        description: "mock",
        inputSchema: undefined,
        execute: async () => ({ text: "assistant reply" }),
      }),
    };
  },
}));

await mock.module("@protean/sandbox-client", () => ({
  DEFAULT_SANDBOX_PROJECT_NAME: "Default",
  createSandboxClient: async () => ({
    sessionId: "sandbox-session-1",
    workspaceMountPath: sandboxClientState.workspaceMountPath,
    workspaceFullPath: "/tmp/sandbox-session-1",
    fs: {
      stat: async () => ({
        size: 0,
        isDirectory: false,
        modified: new Date(0).toISOString(),
        created: new Date(0).toISOString(),
      }),
      readdir: async (dirPath: string) => {
        if (dirPath === "skills" && sandboxClientState.skillDirs.length > 0) {
          return sandboxClientState.skillDirs;
        }
        return [];
      },
      readFile: async (filePath: string) => {
        sandboxClientState.readCalls.push(filePath);
        if (filePath in sandboxClientState.skillFiles) {
          return sandboxClientState.skillFiles[filePath];
        }
        return "remote";
      },
      readFileBuffer: async () => Buffer.from("remote"),
      mkdir: async () => undefined,
      writeFile: async () => undefined,
      writeFileBuffer: async () => undefined,
      move: async () => undefined,
      remove: async () => undefined,
      resolvePath: (filePath: string) => filePath,
    },
    getSession: async () => ({
      sessionId: "sandbox-session-1",
      workspaceMountPath: sandboxClientState.workspaceMountPath,
      workspaceFullPath: "/tmp/sandbox-session-1",
      containerName: "protean-sandbox-sandbox-session-1",
      containerId: "container-1",
      image: "protean-sandbox:1",
      state: "running",
      createdAt: new Date(0).toISOString(),
      exists: true,
      workspaceReady: true,
      containerPresent: true,
      containerRunning: true,
    }),
    exec: async (input: {
      command: string;
      cwd?: string;
      timeoutMs?: number;
    }) => {
      sandboxClientState.execCalls.push(input);
      return {
        exitCode: 0,
        stdout: "remote",
        stderr: "",
        timedOut: false,
        signalCode: null,
      };
    },
    deleteSession: async () => undefined,
  }),
  ensureSandboxProject: async (
    client: { workspaceMountPath: string },
    input?: { name?: string },
  ) => {
    const name = input?.name?.trim() || "Default";
    sandboxClientState.ensuredProjects.push(name);
    return {
      name,
      relativePath: `projects/${name}`,
      workspaceMountPath: `${client.workspaceMountPath}/projects/${name}`,
    };
  },
}));

const { createBashAgent } = await import("./bash-agent");

function buildMessage(role: "user" | "assistant", text: string): UIMessage {
  return {
    id: `${role}-${Math.random().toString(36).slice(2)}`,
    role,
    parts: [
      {
        type: "text",
        text,
      },
    ],
  };
}

async function createMemoryFixture(): Promise<{
  workspaceRoot: string;
  memory: AgentMemory;
}> {
  const workspaceRoot = await mkdtemp(join(tmpdir(), "bash-agent-runtime-"));
  const fs = await createLocalFs(workspaceRoot, noopLogger);
  const memory = await createFsMemory({ fs }, noopLogger);
  return {
    workspaceRoot,
    memory,
  };
}

describe("createBashAgent", () => {
  let workspaceRoot = "";
  let memory: AgentMemory;
  let thread: ThreadRecord;
  const originalApiKey = process.env.OPENROUTER_API_KEY;

  beforeEach(async () => {
    generateCalls = [];
    createAgentCalls = [];
    sandboxClientState.execCalls = [];
    sandboxClientState.ensuredProjects = [];
    sandboxClientState.readCalls = [];
    sandboxClientState.workspaceMountPath = "/workspace";
    sandboxClientState.skillDirs = [];
    sandboxClientState.skillFiles = {};
    const fixture = await createMemoryFixture();
    workspaceRoot = fixture.workspaceRoot;
    memory = fixture.memory;
    thread = await memory.createThread({
      userId: "user-1",
      title: "Bash Agent Thread",
      modelSelection: getDefaultModelSelection(),
    });
    process.env.OPENROUTER_API_KEY = "unused-test-key";
  });

  afterEach(async () => {
    mock.clearAllMocks();

    if (workspaceRoot) {
      await rm(workspaceRoot, { recursive: true, force: true });
    }

    if (typeof originalApiKey === "string") {
      process.env.OPENROUTER_API_KEY = originalApiKey;
    } else {
      delete process.env.OPENROUTER_API_KEY;
    }
  });

  test("loads model selection from the thread record", async () => {
    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "local",
          workspaceRoot,
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    expect(agent.thread.modelSelection).toEqual(thread.modelSelection);
  });

  test("uses active history rather than raw history when generating", async () => {
    const first = await memory.upsertMessage(thread.id, {
      message: buildMessage("user", "hidden old content"),
      modelSelection: thread.modelSelection,
      usage: {
        inputTokens: 100,
        outputTokens: 0,
        totalDurationMs: 1,
      },
    });
    expect(first).not.toBeNull();

    const compacted = await memory.compactIfNeeded(thread.id, {
      policy: {
        maxContextTokens: 1,
        reservedOutputTokens: 0,
      },
      summarizeHistory: async () => buildMessage("user", "summary only"),
    });
    expect(compacted?.didCompact).toBe(true);

    const second = await memory.upsertMessage(thread.id, {
      message: buildMessage("user", "visible message"),
      modelSelection: thread.modelSelection,
      usage: {
        inputTokens: 1,
        outputTokens: 0,
        totalDurationMs: 1,
      },
    });
    expect(second).not.toBeNull();

    const reloaded = await memory.getThreadWithMessages(thread.id);
    expect(reloaded).not.toBeNull();
    expect(deriveActiveHistory(reloaded!).length).toBeLessThan(
      reloaded!.history.length,
    );

    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "local",
          workspaceRoot,
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    await agent.generateThread();

    const serialized = JSON.stringify(generateCalls.at(-1)?.messages ?? []);
    expect(serialized).not.toContain("hidden old content");
    expect(serialized).toContain("visible message");
  });

  test("compacts before generation when context limits are exceeded", async () => {
    const modelInfo = findModel(
      thread.modelSelection.providerId,
      thread.modelSelection.modelId,
    );
    expect(modelInfo).toBeDefined();

    const saved = await memory.upsertMessage(thread.id, {
      message: buildMessage(
        "user",
        "token ".repeat(
          (modelInfo?.contextLimits.total ?? 1) +
            (modelInfo?.contextLimits.maxOutput ?? 0) +
            1_000,
        ),
      ),
      modelSelection: thread.modelSelection,
      usage: {
        inputTokens: 1,
        outputTokens: 0,
        totalDurationMs: 1,
      },
    });
    expect(saved).not.toBeNull();

    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "local",
          workspaceRoot,
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    const result = await agent.generateThread();

    expect(result.thread.lastCompactionOrdinal).not.toBeNull();
  });

  test("persists the assistant reply after generateThread", async () => {
    const saved = await memory.upsertMessage(thread.id, {
      message: buildMessage("user", "hello"),
      modelSelection: thread.modelSelection,
      usage: {
        inputTokens: 1,
        outputTokens: 0,
        totalDurationMs: 1,
      },
    });
    expect(saved).not.toBeNull();

    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "local",
          workspaceRoot,
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    const result = await agent.generateThread();
    const reloaded = await memory.getThreadWithMessages(thread.id);

    expect(result.text).toBe("assistant reply");
    expect(reloaded?.history.at(-1)?.message.role).toBe("assistant");
    expect(
      JSON.stringify(reloaded?.history.at(-1)?.message.parts ?? []),
    ).toContain("assistant reply");
  });

  test("does not require OPENROUTER_API_KEY when modelOverride is provided", async () => {
    delete process.env.OPENROUTER_API_KEY;

    const result = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "local",
          workspaceRoot,
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    expect(result).toBeDefined();
  });

  test("builds sandbox-backed tools when sandbox mode is requested", async () => {
    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "sandbox",
          serviceBaseUrl: "http://sandbox.example",
          serviceToken: "token",
          sessionId: "sandbox-session-1",
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    const bashTool = agent.tools.Bash;
    if (!bashTool?.execute) {
      throw new Error("Bash tool was not configured.");
    }

    const readTool = agent.tools.ReadFile;
    if (!readTool?.execute) {
      throw new Error("ReadFile tool was not configured.");
    }

    await bashTool.execute(
      {
        command: "pwd",
      } as never,
      {} as never,
    );
    await readTool.execute(
      {
        path: "notes.txt",
        withLineNumbers: false,
      } as never,
      {} as never,
    );

    // First exec call is the `rg` availability check at init time, second is the actual `pwd` command.
    const commandCalls = sandboxClientState.execCalls.filter(
      (c) => c.command !== "command -v rg >/dev/null 2>&1",
    );
    expect(commandCalls).toHaveLength(1);
    expect(commandCalls[0]?.command).toBe("pwd");
    expect(sandboxClientState.ensuredProjects).toEqual(["Default"]);
  });

  test("uses remote exec for sandbox globbing", async () => {
    const agent = await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "sandbox",
          serviceBaseUrl: "http://sandbox.example",
          serviceToken: "token",
          sessionId: "sandbox-session-1",
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    const globTool = agent.tools.Glob;
    if (!globTool?.execute) {
      throw new Error("Glob tool was not configured.");
    }

    const result = await globTool.execute(
      {
        pattern: "*.txt",
        maxResults: 10,
      } as never,
      {} as never,
    );

    expect((result as { ok: boolean }).ok).toBe(true);
    expect((result as { matches: string[] }).matches).toEqual([
      "projects/Default/remote",
    ]);
    expect(sandboxClientState.execCalls.at(-1)?.command).toContain(
      "python3 -c",
    );
  });

  test("uses the sandbox service workspace mount path by default", async () => {
    sandboxClientState.workspaceMountPath = "/sandbox-root";

    await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "sandbox",
          serviceBaseUrl: "http://sandbox.example",
          serviceToken: "token",
          sessionId: "sandbox-session-1",
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    expect(createAgentCalls.at(-1)?.instructions).toContain("/sandbox-root");
  });

  test("uses the thread project name by default in sandbox mode", async () => {
    const projectThread = await memory.createThread({
      userId: "user-1",
      title: "Project Thread",
      projectName: "My Project",
      modelSelection: getDefaultModelSelection(),
    });

    await createBashAgent(
      {
        threadId: projectThread.id,
        memory,
        environment: {
          kind: "sandbox",
          serviceBaseUrl: "http://sandbox.example",
          serviceToken: "token",
          sessionId: "sandbox-session-1",
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    expect(sandboxClientState.ensuredProjects).toEqual(["My Project"]);
    expect(createAgentCalls.at(-1)?.instructions).toContain("/workspace");
  });

  test("preserves explicit sandbox workspaceRoot overrides", async () => {
    await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "sandbox",
          serviceBaseUrl: "http://sandbox.example",
          serviceToken: "token",
          sessionId: "sandbox-session-1",
          workspaceRoot: "/workspace/custom-root",
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    expect(sandboxClientState.ensuredProjects).toEqual([]);
    expect(createAgentCalls.at(-1)?.instructions).toContain(
      "/workspace/custom-root",
    );
  });

  test("discovers skills and includes them in sandbox agent instructions", async () => {
    sandboxClientState.skillDirs = [
      { name: "code-review", isDirectory: true },
    ];
    sandboxClientState.skillFiles = {
      "skills/code-review/SKILL.md": `---
name: Code Review
description: Automated code review skill
---
# Code Review Skill`,
    };

    await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "sandbox",
          serviceBaseUrl: "http://sandbox.example",
          serviceToken: "token",
          sessionId: "sandbox-session-1",
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    const instructions = createAgentCalls.at(-1)?.instructions ?? "";
    expect(instructions).toContain("Available Skills");
    expect(instructions).toContain("Code Review");
    expect(instructions).toContain("Automated code review skill");
    expect(instructions).toContain("skills/code-review/SKILL.md");
  });

  test("local mode produces no skills in prompt", async () => {
    await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "local",
          workspaceRoot,
        },
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    const instructions = createAgentCalls.at(-1)?.instructions ?? "";
    expect(instructions).not.toContain("Available Skills");
  });

  test("injects additional non-colliding tools", async () => {
    const additionalTools: Record<string, Tool> = {
      ExtraTool: {
        description: "Extra test tool",
        inputSchema: undefined,
        execute: async () => ({ ok: true }),
      } as unknown as Tool,
    };

    await createBashAgent(
      {
        threadId: thread.id,
        memory,
        environment: {
          kind: "local",
          workspaceRoot,
        },
        additionalTools,
        modelOverride: {} as LanguageModel,
      },
      noopLogger,
    );

    expect(createAgentCalls.at(-1)?.tools?.ExtraTool).toBeDefined();
  });

  test("rejects additional tools that collide with internal tools", async () => {
    expect(
      createBashAgent(
        {
          threadId: thread.id,
          memory,
          environment: {
            kind: "local",
            workspaceRoot,
          },
          additionalTools: {
            Bash: {
              description: "conflict",
              inputSchema: undefined,
              execute: async () => ({ ok: false }),
            } as unknown as Tool,
          },
          modelOverride: {} as LanguageModel,
        },
        noopLogger,
      ),
    ).rejects.toThrow("Tool name collision");
  });
});
