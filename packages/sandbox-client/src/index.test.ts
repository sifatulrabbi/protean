import { afterEach, describe, expect, test } from "bun:test";

import {
  createSandboxClient,
  createSandboxSession,
  ensureSandboxSession,
  ensureSandboxProject,
  listSandboxProjects,
} from "./index";

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
});

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("sandbox-client", () => {
  test("creates a sandbox session", async () => {
    const calls: Array<{ url: string; method: string }> = [];

    globalThis.fetch = (async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      calls.push({ url, method });

      return jsonResponse(200, {
        ok: true,
        data: {
          sessionId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
          workspaceMountPath: "/workspace",
          workspaceFullPath: "/tmp/sessions/01ARZ3NDEKTSV4RRFFQ69G5FAV",
          containerName: "protean-sandbox-01ARZ3NDEKTSV4RRFFQ69G5FAV",
          containerId: "container-1",
          image: "protean-sandbox:1",
          state: "running",
          createdAt: "2026-03-01T00:00:00.000Z",
        },
      });
    }) as typeof fetch;

    const session = await createSandboxSession({
      baseUrl: "http://sandbox.example/",
      serviceToken: "token",
    });

    expect(session.sessionId).toBe("01ARZ3NDEKTSV4RRFFQ69G5FAV");
    expect(calls).toEqual([
      {
        url: "http://sandbox.example/api/v1/sandbox/sessions",
        method: "POST",
      },
    ]);
  });

  test("creates a sandbox session with explicit session id", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];

    globalThis.fetch = (async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, body });

      return jsonResponse(200, {
        ok: true,
        data: {
          sessionId: "alice@example.com",
          workspaceMountPath: "/workspace",
          workspaceFullPath: "/tmp/sessions/alice@example.com",
          containerName: "protean-sandbox-alice",
          containerId: "container-1",
          image: "protean-sandbox:1",
          state: "running",
          createdAt: "2026-03-01T00:00:00.000Z",
        },
      });
    }) as typeof fetch;

    await createSandboxSession(
      {
        baseUrl: "http://sandbox.example/",
        serviceToken: "token",
      },
      { sessionId: "alice@example.com" },
    );

    expect(calls[0]).toEqual({
      url: "http://sandbox.example/api/v1/sandbox/sessions",
      method: "POST",
      body: '{"sessionId":"alice@example.com"}',
    });
  });

  test("ensureSandboxSession reuses an existing session", async () => {
    const calls: string[] = [];
    globalThis.fetch = (async (input) => {
      const url = String(input);
      calls.push(url);

      if (url.endsWith("/api/v1/sandbox/sessions/alice%40example.com")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            sessionId: "alice@example.com",
            workspaceMountPath: "/workspace",
            workspaceFullPath: "/tmp/sandbox-session-alice",
            containerName: "protean-sandbox-alice",
            containerId: "container-1",
            image: "protean-sandbox:1",
            state: "running",
            createdAt: "2026-03-01T00:00:00.000Z",
            exists: true,
            workspaceReady: true,
            containerPresent: true,
            containerRunning: true,
          },
        });
      }

      return jsonResponse(500, {
        ok: false,
        error: {
          code: "INTERNAL",
          message: `unexpected call: ${url}`,
        },
      });
    }) as typeof fetch;

    const client = await ensureSandboxSession(
      {
        baseUrl: "http://sandbox.example/",
        serviceToken: "token",
      },
      { sessionId: "alice@example.com" },
    );

    expect(client.sessionId).toBe("alice@example.com");
    expect(
      calls.filter((url) =>
        url.endsWith("/api/v1/sandbox/sessions/alice%40example.com"),
      ),
    ).toHaveLength(1);
  });

  test("ensureSandboxSession creates session when missing", async () => {
    let fetchCount = 0;
    globalThis.fetch = (async (input, init) => {
      fetchCount += 1;
      const url = String(input);

      if (
        url.endsWith("/api/v1/sandbox/sessions/alice%40example.com") &&
        fetchCount === 1
      ) {
        return jsonResponse(404, {
          ok: false,
          error: {
            code: "NOT_FOUND",
            message: "sandbox session not found",
          },
        });
      }

      if (
        url.endsWith("/api/v1/sandbox/sessions") &&
        (init?.method ?? "GET") === "POST"
      ) {
        return jsonResponse(200, {
          ok: true,
          data: {
            sessionId: "alice@example.com",
            workspaceMountPath: "/workspace",
            workspaceFullPath: "/tmp/sandbox-session-alice",
            containerName: "protean-sandbox-alice",
            containerId: "container-1",
            image: "protean-sandbox:1",
            state: "running",
            createdAt: "2026-03-01T00:00:00.000Z",
          },
        });
      }

      if (url.endsWith("/api/v1/sandbox/sessions/alice%40example.com")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            sessionId: "alice@example.com",
            workspaceMountPath: "/workspace",
            workspaceFullPath: "/tmp/sandbox-session-alice",
            containerName: "protean-sandbox-alice",
            containerId: "container-1",
            image: "protean-sandbox:1",
            state: "running",
            createdAt: "2026-03-01T00:00:00.000Z",
            exists: true,
            workspaceReady: true,
            containerPresent: true,
            containerRunning: true,
          },
        });
      }

      return jsonResponse(500, {
        ok: false,
        error: {
          code: "INTERNAL",
          message: `unexpected call: ${url}`,
        },
      });
    }) as typeof fetch;

    const client = await ensureSandboxSession(
      {
        baseUrl: "http://sandbox.example/",
        serviceToken: "token",
      },
      { sessionId: "alice@example.com" },
    );

    expect(client.sessionId).toBe("alice@example.com");
  });

  test("returns a stable fs implementation bound to the session", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];

    // Mocking the fetch call from the fs client
    globalThis.fetch = (async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, body });

      if (url.endsWith("/api/v1/sandbox/sessions/session-1")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            sessionId: "session-1",
            workspaceMountPath: "/sandbox-root",
            workspaceFullPath: "/tmp/sandbox-session-1",
            containerName: "protean-sandbox-session-1",
            containerId: "container-1",
            image: "protean-sandbox:1",
            state: "running",
            createdAt: "2026-03-01T00:00:00.000Z",
            exists: true,
            workspaceReady: true,
            containerPresent: true,
            containerRunning: true,
          },
        });
      }

      if (url.endsWith("/files/read?path=docs%2Fnote.txt")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            content: "hello",
          },
        });
      }

      if (url.endsWith("/exec")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            exitCode: 0,
            stdout: "ok",
            stderr: "",
            timedOut: false,
            signalCode: null,
          },
        });
      }

      if (url.endsWith("/files/rename")) {
        return jsonResponse(200, {
          ok: true,
          data: { renamed: true },
        });
      }

      return jsonResponse(500, {
        ok: false,
        error: {
          code: "INTERNAL",
          message: `unexpected call: ${url} ${method}`,
        },
      });
    }) as typeof fetch;

    const client = await createSandboxClient({
      baseUrl: "http://sandbox.example",
      serviceToken: "token",
      sessionId: "session-1",
    });

    expect(client.workspaceMountPath).toBe("/sandbox-root");
    expect(client.workspaceFullPath).toBe("/tmp/sandbox-session-1");
    expect(client.fs.resolvePath("docs/note.txt")).toBe(
      "/sandbox-root/docs/note.txt",
    );
    expect(client.fs.readFile("docs/note.txt")).resolves.toBe("hello");
    expect(
      client.fs.move("docs\\note.txt", "/archive/note.txt"),
    ).resolves.toBeUndefined();
    expect(client.exec({ command: "pwd" })).resolves.toMatchObject({
      exitCode: 0,
      stdout: "ok",
    });

    expect(calls[0]?.url).toBe(
      "http://sandbox.example/api/v1/sandbox/sessions/session-1",
    );
    expect(calls[1]?.url).toBe(
      "http://sandbox.example/api/v1/sandbox/sessions/session-1/files/read?path=docs%2Fnote.txt",
    );
    const moveCall = calls.find((call) => call.url.endsWith("/files/rename"));
    expect(moveCall?.body).toContain('"path":"docs/note.txt"');
    expect(moveCall?.body).toContain('"newPath":"archive/note.txt"');
  });

  test("creates and lists sandbox projects", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];

    globalThis.fetch = (async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, body });

      if (url.endsWith("/api/v1/sandbox/sessions/session-1")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            sessionId: "session-1",
            workspaceMountPath: "/workspace",
            workspaceFullPath: "/tmp/sandbox-session-1",
            containerName: "protean-sandbox-session-1",
            containerId: "container-1",
            image: "protean-sandbox:1",
            state: "running",
            createdAt: "2026-03-01T00:00:00.000Z",
            exists: true,
            workspaceReady: true,
            containerPresent: true,
            containerRunning: true,
          },
        });
      }

      if (url.endsWith("/files/stat?path=projects")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            size: 0,
            isDirectory: true,
            modified: "2026-03-01T00:00:00.000Z",
            created: "2026-03-01T00:00:00.000Z",
          },
        });
      }

      if (url.endsWith("/files/mkdir")) {
        return jsonResponse(200, {
          ok: true,
          data: {},
        });
      }

      if (url.endsWith("/files/readdir?path=projects")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            entries: [
              { name: "My Project", isDirectory: true },
              { name: "bad/name", isDirectory: true },
              { name: "README.md", isDirectory: false },
              { name: "Zeta", isDirectory: true },
            ],
          },
        });
      }

      return jsonResponse(500, {
        ok: false,
        error: {
          code: "INTERNAL",
          message: `unexpected call: ${url} ${method}`,
        },
      });
    }) as typeof fetch;

    const client = await createSandboxClient({
      baseUrl: "http://sandbox.example",
      serviceToken: "token",
      sessionId: "session-1",
    });

    const project = await ensureSandboxProject(client, {
      name: "My Project",
    });
    expect(project).toEqual({
      name: "My Project",
      relativePath: "projects/My Project",
      workspaceMountPath: "/workspace/projects/My Project",
    });

    const projects = await listSandboxProjects(client);
    expect(projects.map((item) => item.name)).toEqual(["My Project", "Zeta"]);

    const mkdirCalls = calls.filter((call) => call.url.endsWith("/files/mkdir"));
    expect(mkdirCalls).toHaveLength(2);
    expect(mkdirCalls[0]?.body).toContain('"path":"projects"');
    expect(mkdirCalls[1]?.body).toContain('"path":"projects/My Project"');
  });

  test("treats existing project directories as ensured", async () => {
    globalThis.fetch = (async (input, init) => {
      const url = String(input);

      if (url.endsWith("/api/v1/sandbox/sessions/session-1")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            sessionId: "session-1",
            workspaceMountPath: "/workspace",
            workspaceFullPath: "/tmp/sandbox-session-1",
            containerName: "protean-sandbox-session-1",
            containerId: "container-1",
            image: "protean-sandbox:1",
            state: "running",
            createdAt: "2026-03-01T00:00:00.000Z",
            exists: true,
            workspaceReady: true,
            containerPresent: true,
            containerRunning: true,
          },
        });
      }

      if (url.endsWith("/files/mkdir")) {
        const payload =
          typeof init?.body === "string"
            ? (JSON.parse(init.body) as { path?: string })
            : {};
        if (payload.path === "projects" || payload.path === "projects/Default") {
          return jsonResponse(409, {
            ok: false,
            error: {
              code: "ALREADY_EXISTS",
              message: "already exists",
            },
          });
        }

        return jsonResponse(200, {
          ok: true,
          data: {},
        });
      }

      if (url.endsWith("/files/stat?path=projects")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            size: 0,
            isDirectory: true,
            modified: "2026-03-01T00:00:00.000Z",
            created: "2026-03-01T00:00:00.000Z",
          },
        });
      }

      if (url.endsWith("/files/stat?path=projects%2FDefault")) {
        return jsonResponse(200, {
          ok: true,
          data: {
            size: 0,
            isDirectory: true,
            modified: "2026-03-01T00:00:00.000Z",
            created: "2026-03-01T00:00:00.000Z",
          },
        });
      }

      return jsonResponse(500, {
        ok: false,
        error: {
          code: "INTERNAL",
          message: `unexpected call: ${url}`,
        },
      });
    }) as typeof fetch;

    const client = await createSandboxClient({
      baseUrl: "http://sandbox.example",
      serviceToken: "token",
      sessionId: "session-1",
    });

    await expect(
      ensureSandboxProject(client, { name: "Default" }),
    ).resolves.toEqual({
      name: "Default",
      relativePath: "projects/Default",
      workspaceMountPath: "/workspace/projects/Default",
    });
  });

  test("rejects invalid sandbox project names", async () => {
    globalThis.fetch = (async (_input, _init) =>
      jsonResponse(200, {
        ok: true,
        data: {
          sessionId: "session-1",
          workspaceMountPath: "/workspace",
          workspaceFullPath: "/tmp/sandbox-session-1",
          containerName: "protean-sandbox-session-1",
          containerId: "container-1",
          image: "protean-sandbox:1",
          state: "running",
          createdAt: "2026-03-01T00:00:00.000Z",
          exists: true,
          workspaceReady: true,
          containerPresent: true,
          containerRunning: true,
        },
      })) as typeof fetch;

    const client = await createSandboxClient({
      baseUrl: "http://sandbox.example",
      serviceToken: "token",
      sessionId: "session-1",
    });

    await expect(
      ensureSandboxProject(client, { name: "bad/name" }),
    ).rejects.toThrow("Invalid project name");
  });
});
