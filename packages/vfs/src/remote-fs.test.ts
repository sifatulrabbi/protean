import { afterEach, describe, expect, test } from "bun:test";
import { createRemoteFs } from "./remote-fs";

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

describe("createRemoteFs", () => {
  test("maps VFS NOT_FOUND to ENOENT for readFile", async () => {
    globalThis.fetch = (async (input, init) => {
      const url = String(input);

      if (url.endsWith("/api/v1/files/mkdir")) {
        return jsonResponse(200, { ok: true, data: { created: true } });
      }

      if (url.includes("/api/v1/files/read?")) {
        return jsonResponse(404, {
          ok: false,
          error: {
            code: "NOT_FOUND",
            message: "file not found",
          },
        });
      }

      return jsonResponse(500, {
        ok: false,
        error: {
          code: "INTERNAL",
          message: `unexpected call: ${url} ${init?.method ?? "GET"}`,
        },
      });
    }) as typeof fetch;

    const fs = await createRemoteFs({
      baseUrl: "http://vfs.example",
      serviceToken: "token",
      userId: "user-12345678",
    });

    await expect(
      fs.readFile(".threads/thread.missing.json"),
    ).rejects.toMatchObject({
      code: "ENOENT",
    });
  });

  test("prepares workspace during initialization", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    globalThis.fetch = (async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? init.body : undefined;

      calls.push({ url, method, body });

      if (url.endsWith("/api/v1/files/mkdir")) {
        return jsonResponse(200, { ok: true, data: { created: true } });
      }

      if (url.includes("/api/v1/files/readdir?")) {
        return jsonResponse(200, {
          ok: true,
          data: { entries: [] },
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

    const fs = await createRemoteFs({
      baseUrl: "http://vfs.example",
      serviceToken: "token",
      userId: "user-12345678",
    });

    // Workspace preparation should be the first request.
    expect(calls[0]?.url).toBe("http://vfs.example/api/v1/files/mkdir");
    expect(calls[0]?.method).toBe("POST");
    expect(calls[0]?.body).toContain('"path":"."');

    await expect(fs.readdir(".")).resolves.toEqual([]);
  });

  test("uses rename endpoint for move with normalized paths", async () => {
    const calls: Array<{ url: string; method: string; body?: string }> = [];
    globalThis.fetch = (async (input, init) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, body });

      if (url.endsWith("/api/v1/files/mkdir")) {
        return jsonResponse(200, { ok: true, data: { created: true } });
      }

      if (url.endsWith("/api/v1/files/rename")) {
        return jsonResponse(200, { ok: true, data: { renamed: true } });
      }

      return jsonResponse(500, {
        ok: false,
        error: {
          code: "INTERNAL",
          message: `unexpected call: ${url} ${method}`,
        },
      });
    }) as typeof fetch;

    const fs = await createRemoteFs({
      baseUrl: "http://vfs.example",
      serviceToken: "token",
      userId: "user-12345678",
    });

    await fs.move("docs\\old.txt", "/archive/new.txt");

    const moveCall = calls.find((c) => c.url.endsWith("/api/v1/files/rename"));
    expect(moveCall).toBeDefined();
    expect(moveCall?.method).toBe("PATCH");
    expect(moveCall?.body).toContain('"path":"docs/old.txt"');
    expect(moveCall?.body).toContain('"newPath":"archive/new.txt"');
  });
});
