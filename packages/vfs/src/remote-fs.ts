import { normalize } from "path";
import { type Logger } from "@protean/logger";
import { type FS } from "./interfaces";

interface RemoteFsConfig {
  baseUrl: string;
  serviceToken: string;
  userId: string;
  logger?: Logger;
}

interface VfsEnvelope<T> {
  ok: boolean;
  data?: T;
  error?: {
    code?: string;
    message?: string;
  };
}

function mapErrnoCode(
  status: number,
  vfsCode?: string,
): NodeJS.ErrnoException["code"] | undefined {
  if (vfsCode === "NOT_FOUND" || status === 404) {
    return "ENOENT";
  }

  if (vfsCode === "PATH_TRAVERSAL" || status === 403) {
    return "EACCES";
  }

  return undefined;
}

function buildVfsError(
  res: Response,
  envelope?: VfsEnvelope<unknown> | null,
): Error {
  const vfsCode = envelope?.error?.code;
  const vfsMessage = envelope?.error?.message;
  const fallbackMessage = `VFS request failed: ${res.status} ${res.statusText}`;
  const resolvedMessage = vfsMessage ?? fallbackMessage;
  const message = vfsCode ? `${vfsCode}: ${resolvedMessage}` : resolvedMessage;

  const error = new Error(message) as NodeJS.ErrnoException & {
    status?: number;
    statusText?: string;
    vfsCode?: string;
  };

  error.name = "VfsRequestError";
  error.status = res.status;
  error.statusText = res.statusText;
  error.vfsCode = vfsCode;

  const errnoCode = mapErrnoCode(res.status, vfsCode);
  if (errnoCode) {
    error.code = errnoCode;
  }

  return error;
}

export async function createRemoteFs(config: RemoteFsConfig): Promise<FS> {
  const { baseUrl, serviceToken, userId, logger } = config;
  const base = baseUrl.replace(/\/+$/, "");

  function headers(): Record<string, string> {
    return {
      "Authorization": `Bearer ${serviceToken}`,
      "X-User-Id": userId,
    };
  }

  async function request(url: string, init?: RequestInit): Promise<Response> {
    return fetch(url, {
      ...init,
      headers: { ...headers(), ...init?.headers },
    });
  }

  async function parseEnvelope<T>(
    res: Response,
  ): Promise<VfsEnvelope<T> | null> {
    const contentType = res.headers.get("content-type") ?? "";
    if (!contentType.includes("application/json")) {
      return null;
    }

    try {
      return (await res.json()) as VfsEnvelope<T>;
    } catch {
      return null;
    }
  }

  async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
    const res = await request(url, init);
    const envelope = await parseEnvelope<T>(res);
    if (!res.ok) {
      throw buildVfsError(res, envelope);
    }

    if (!envelope) {
      throw new Error("VFS request returned a non-JSON response.");
    }

    const body = envelope;
    if (!body.ok) {
      throw buildVfsError(res, body);
    }

    return body.data as T;
  }

  function normalizePath(filePath: string): string {
    return normalize(filePath).replace(/^\/+/, "").replace(/\\/g, "/");
  }

  logger?.debug("RemoteFS.ensureWorkspace", { userId });
  await fetchJson(`${base}/api/v1/files/mkdir`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path: "." }),
  });

  return {
    stat: async (filePath) => {
      logger?.debug("RemoteFS.stat", { filePath });
      const q = encodeURIComponent(normalizePath(filePath));
      return fetchJson(`${base}/api/v1/files/stat?path=${q}`);
    },

    readdir: async (dirPath) => {
      logger?.debug("RemoteFS.readdir", { dirPath });
      const q = encodeURIComponent(normalizePath(dirPath));
      const data = await fetchJson<{
        entries: Array<{ name: string; isDirectory: boolean }>;
      }>(`${base}/api/v1/files/readdir?path=${q}`);
      return data.entries;
    },

    readFile: async (filePath) => {
      logger?.debug("RemoteFS.readFile", { filePath });
      const q = encodeURIComponent(normalizePath(filePath));
      const data = await fetchJson<{ content: string }>(
        `${base}/api/v1/files/read?path=${q}`,
      );
      return data.content;
    },

    readFileBuffer: async (filePath) => {
      logger?.debug("RemoteFS.readFileBuffer", { filePath });
      const q = encodeURIComponent(normalizePath(filePath));
      const res = await request(`${base}/api/v1/files/read-binary?path=${q}`);
      if (!res.ok) {
        const envelope = await parseEnvelope(res);
        throw buildVfsError(res, envelope);
      }
      const ab = await res.arrayBuffer();
      return Buffer.from(ab);
    },

    mkdir: async (dirPath) => {
      logger?.debug("RemoteFS.mkdir", { dirPath });
      await fetchJson(`${base}/api/v1/files/mkdir`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: normalizePath(dirPath) }),
      });
    },

    writeFile: async (filePath, content) => {
      logger?.debug("RemoteFS.writeFile", { filePath, bytes: content.length });
      await fetchJson(`${base}/api/v1/files/write`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: normalizePath(filePath), content }),
      });
    },

    writeFileBuffer: async (filePath, content) => {
      logger?.debug("RemoteFS.writeFileBuffer", {
        filePath,
        bytes: content.length,
      });
      const formData = new FormData();
      formData.append("path", normalizePath(filePath));
      const blobPayload = new Uint8Array(content.length);
      blobPayload.set(content);
      formData.append("file", new Blob([blobPayload]));
      const res = await request(`${base}/api/v1/files/write-binary`, {
        method: "POST",
        body: formData,
      });
      const envelope = await parseEnvelope(res);
      if (!res.ok) {
        throw buildVfsError(res, envelope);
      }
      if (!envelope) {
        throw new Error("VFS request returned a non-JSON response.");
      }
      if (!envelope.ok) {
        throw buildVfsError(res, envelope);
      }
    },

    move: async (sourcePath, destinationPath) => {
      logger?.debug("RemoteFS.move", { sourcePath, destinationPath });
      await fetchJson(`${base}/api/v1/files/rename`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          path: normalizePath(sourcePath),
          newPath: normalizePath(destinationPath),
        }),
      });
    },

    remove: async (fullPath) => {
      logger?.debug("RemoteFS.remove", { fullPath });
      const q = encodeURIComponent(normalizePath(fullPath));
      await fetchJson(`${base}/api/v1/files/remove?path=${q}`, {
        method: "DELETE",
      });
    },

    resolvePath: (filePath) => {
      return normalizePath(filePath);
    },
  };
}
