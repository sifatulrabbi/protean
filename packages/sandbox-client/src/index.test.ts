import { afterEach, describe, expect, test } from "bun:test";

import { createSandboxClient, createSandboxSession } from "./index";

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
});
