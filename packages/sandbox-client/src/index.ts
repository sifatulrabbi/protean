import { normalize } from "node:path";
import type { Logger } from "@protean/logger";
import type { FS } from "@protean/vfs";

export const DEFAULT_SANDBOX_PROJECT_NAME = "Default";

const SANDBOX_PROJECTS_DIR = "projects";
const CONTROL_CHARACTERS = /[\u0000-\u001f\u007f]/;

export interface SandboxSession {
  sessionId: string;
  workspaceMountPath: string;
  workspaceFullPath: string;
  containerName: string;
  containerId: string;
  image: string;
  state: string;
  createdAt: string;
}

export interface SandboxSessionStatus {
  sessionId: string;
  workspaceMountPath: string;
  workspaceFullPath: string;
  containerName: string;
  containerId?: string;
  image: string;
  state: string;
  createdAt: string;
  exists: boolean;
  workspaceReady: boolean;
  containerPresent: boolean;
  containerRunning: boolean;
}

export interface SandboxExecResult {
  exitCode: number;
  stdout: string;
  stderr: string;
  timedOut: boolean;
  signalCode: string | null;
}

export interface SandboxServiceClientConfig {
  baseUrl: string;
  serviceToken: string;
  logger?: Logger;
}

export interface CreateSandboxSessionInput {
  sessionId?: string;
}

export interface SandboxClientConfig extends SandboxServiceClientConfig {
  sessionId: string;
}

export interface SandboxClient {
  sessionId: string;
  workspaceMountPath: string;
  workspaceFullPath: string;
  fs: FS;
  getSession(): Promise<SandboxSessionStatus>;
  exec(input: {
    command: string;
    cwd?: string;
    timeoutMs?: number;
  }): Promise<SandboxExecResult>;
  deleteSession(): Promise<void>;
}

export interface SandboxProject {
  name: string;
  relativePath: string;
  workspaceMountPath: string;
}

export interface WorkspaceProjectContext {
  fs: FS;
  workspaceMountPath: string;
}

interface VfsEnvelope<T> {
  ok: boolean;
  data?: T;
  error?: {
    code?: string;
    message?: string;
  };
}

function normalizeBaseUrl(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, "");
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

function buildSandboxError(
  res: Response,
  envelope?: VfsEnvelope<unknown> | null,
): Error {
  const vfsCode = envelope?.error?.code;
  const vfsMessage = envelope?.error?.message;
  const fallbackMessage = `Sandbox request failed: ${res.status} ${res.statusText}`;
  const resolvedMessage = vfsMessage ?? fallbackMessage;
  const message = vfsCode ? `${vfsCode}: ${resolvedMessage}` : resolvedMessage;

  const error = new Error(message) as NodeJS.ErrnoException & {
    status?: number;
    statusText?: string;
    vfsCode?: string;
  };

  error.name = "SandboxRequestError";
  error.status = res.status;
  error.statusText = res.statusText;
  error.vfsCode = vfsCode;

  const errnoCode = mapErrnoCode(res.status, vfsCode);
  if (errnoCode) {
    error.code = errnoCode;
  }

  return error;
}

function normalizePath(filePath: string): string {
  return normalize(filePath).replace(/^\/+/, "").replace(/\\/g, "/");
}

function validateProjectName(name: string): string {
  const trimmed = name.trim();
  if (!trimmed) {
    throw new Error("Project name must not be empty.");
  }
  if (trimmed === "." || trimmed === "..") {
    throw new Error(`Invalid project name: "${name}".`);
  }
  if (
    trimmed.includes("/") ||
    trimmed.includes("\\") ||
    CONTROL_CHARACTERS.test(trimmed)
  ) {
    throw new Error(`Invalid project name: "${name}".`);
  }

  return trimmed;
}

function toProjectRelativePath(name: string): string {
  return `${SANDBOX_PROJECTS_DIR}/${validateProjectName(name)}`;
}

function toProjectMountPath(workspaceMountPath: string, name: string): string {
  return resolveWorkspacePath(workspaceMountPath, toProjectRelativePath(name));
}

async function ensureDirectory(fs: FS, dirPath: string): Promise<void> {
  try {
    await fs.mkdir(dirPath);
    return;
  } catch {
    const stat = await fs.stat(dirPath);
    if (stat.isDirectory) {
      return;
    }
    throw new Error(`Path "${dirPath}" already exists and is not a directory.`);
  }
}

function resolveWorkspacePath(workspaceRoot: string, filePath: string): string {
  const normalizedRoot = workspaceRoot.replace(/\/+$/, "") || "/";
  const normalizedPath = normalizePath(filePath);

  if (normalizedPath === ".") {
    return normalizedRoot;
  }

  return `${normalizedRoot}/${normalizedPath}`;
}

function createRequester(config: SandboxServiceClientConfig) {
  const baseUrl = normalizeBaseUrl(config.baseUrl);

  function headers(extra?: Record<string, string>): Record<string, string> {
    return {
      Authorization: `Bearer ${config.serviceToken}`,
      ...extra,
    };
  }

  async function request(url: string, init?: RequestInit): Promise<Response> {
    config.logger?.debug("sandbox-client.request", {
      url,
      method: init?.method ?? "GET",
    });

    return fetch(url, {
      ...init,
      headers: {
        ...headers(),
        ...(init?.headers ?? {}),
      },
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
      throw buildSandboxError(res, envelope);
    }

    if (!envelope) {
      throw new Error("Sandbox request returned a non-JSON response.");
    }

    if (!envelope.ok) {
      throw buildSandboxError(res, envelope);
    }

    return envelope.data as T;
  }

  return {
    baseUrl,
    request,
    fetchJson,
    parseEnvelope,
  };
}

function createSandboxFs(
  config: SandboxServiceClientConfig & {
    sessionId: string;
    workspaceMountPath: string;
  },
): FS {
  const requester = createRequester(config);
  const basePath = `${requester.baseUrl}/api/v1/sandbox/sessions/${encodeURIComponent(config.sessionId)}/files`;

  return {
    stat: async (filePath) => {
      const q = encodeURIComponent(normalizePath(filePath));
      return requester.fetchJson(`${basePath}/stat?path=${q}`);
    },

    readdir: async (dirPath) => {
      const q = encodeURIComponent(normalizePath(dirPath));
      const data = await requester.fetchJson<{
        entries: Array<{ name: string; isDirectory: boolean }>;
      }>(`${basePath}/readdir?path=${q}`);
      return data.entries;
    },

    readFile: async (filePath) => {
      const q = encodeURIComponent(normalizePath(filePath));
      const data = await requester.fetchJson<{ content: string }>(
        `${basePath}/read?path=${q}`,
      );
      return data.content;
    },

    readFileBuffer: async (filePath) => {
      const q = encodeURIComponent(normalizePath(filePath));
      const res = await requester.request(`${basePath}/read-binary?path=${q}`);
      if (!res.ok) {
        const envelope = await requester.parseEnvelope(res);
        throw buildSandboxError(res, envelope);
      }

      return Buffer.from(await res.arrayBuffer());
    },

    mkdir: async (dirPath) => {
      await requester.fetchJson(`${basePath}/mkdir`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          path: normalizePath(dirPath),
        }),
      });
    },

    writeFile: async (filePath, content) => {
      await requester.fetchJson(`${basePath}/write`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          path: normalizePath(filePath),
          content,
        }),
      });
    },

    writeFileBuffer: async (filePath, content) => {
      const formData = new FormData();
      formData.append("path", normalizePath(filePath));
      const blobPayload = new Uint8Array(content.length);
      blobPayload.set(content);
      formData.append("file", new Blob([blobPayload]));

      const res = await requester.request(`${basePath}/write-binary`, {
        method: "POST",
        body: formData,
      });
      const envelope = await requester.parseEnvelope(res);
      if (!res.ok) {
        throw buildSandboxError(res, envelope);
      }
      if (!envelope?.ok) {
        throw buildSandboxError(res, envelope);
      }
    },

    move: async (sourcePath, destinationPath) => {
      await requester.fetchJson(`${basePath}/rename`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          path: normalizePath(sourcePath),
          newPath: normalizePath(destinationPath),
        }),
      });
    },

    remove: async (fullPath) => {
      const q = encodeURIComponent(normalizePath(fullPath));
      await requester.fetchJson(`${basePath}/remove?path=${q}`, {
        method: "DELETE",
      });
    },

    resolvePath: (filePath) =>
      resolveWorkspacePath(config.workspaceMountPath, filePath),
  };
}

export async function createSandboxSession(
  config: SandboxServiceClientConfig,
  input?: CreateSandboxSessionInput,
): Promise<SandboxSession> {
  const requester = createRequester(config);
  const hasSessionId = typeof input?.sessionId === "string";
  return requester.fetchJson<SandboxSession>(
    `${requester.baseUrl}/api/v1/sandbox/sessions`,
    {
      method: "POST",
      ...(hasSessionId
        ? {
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ sessionId: input?.sessionId }),
          }
        : {}),
    },
  );
}

function isSessionNotFoundError(error: unknown): boolean {
  if (!error || typeof error !== "object") {
    return false;
  }

  const maybeErr = error as NodeJS.ErrnoException & {
    status?: number;
    vfsCode?: string;
  };
  return (
    maybeErr.status === 404 ||
    maybeErr.code === "ENOENT" ||
    maybeErr.vfsCode === "NOT_FOUND"
  );
}

export async function ensureSandboxSession(
  config: SandboxServiceClientConfig,
  input: { sessionId: string },
): Promise<SandboxClient> {
  try {
    return await createSandboxClient({
      ...config,
      sessionId: input.sessionId,
    });
  } catch (error) {
    if (!isSessionNotFoundError(error)) {
      throw error;
    }
  }

  await createSandboxSession(config, { sessionId: input.sessionId });
  return createSandboxClient({
    ...config,
    sessionId: input.sessionId,
  });
}

export async function createSandboxClient(
  config: SandboxClientConfig,
): Promise<SandboxClient> {
  const requester = createRequester(config);
  const basePath = `${requester.baseUrl}/api/v1/sandbox/sessions/${encodeURIComponent(config.sessionId)}`;
  const session = await requester.fetchJson<SandboxSessionStatus>(basePath);
  const fs = createSandboxFs({
    ...config,
    workspaceMountPath: session.workspaceMountPath,
  });

  return {
    sessionId: session.sessionId,
    workspaceMountPath: session.workspaceMountPath,
    workspaceFullPath: session.workspaceFullPath,
    fs,
    getSession: async () => requester.fetchJson<SandboxSessionStatus>(basePath),
    exec: async ({ command, cwd, timeoutMs }) =>
      requester.fetchJson<SandboxExecResult>(`${basePath}/exec`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          command,
          cwd,
          timeoutMs,
        }),
      }),
    deleteSession: async () => {
      await requester.fetchJson<{ deleted: boolean }>(basePath, {
        method: "DELETE",
      });
    },
  };
}

export async function ensureSandboxProject(
  client: SandboxClient,
  input?: { name?: string },
): Promise<SandboxProject> {
  return ensureWorkspaceProject(
    {
      fs: client.fs,
      workspaceMountPath: client.workspaceMountPath,
    },
    input,
  );
}

export async function ensureWorkspaceProject(
  context: WorkspaceProjectContext,
  input?: { name?: string },
): Promise<SandboxProject> {
  const name = validateProjectName(input?.name ?? DEFAULT_SANDBOX_PROJECT_NAME);
  await ensureDirectory(context.fs, SANDBOX_PROJECTS_DIR);

  const relativePath = toProjectRelativePath(name);
  await ensureDirectory(context.fs, relativePath);

  return {
    name,
    relativePath,
    workspaceMountPath: toProjectMountPath(context.workspaceMountPath, name),
  };
}

export async function listSandboxProjects(
  client: SandboxClient,
): Promise<SandboxProject[]> {
  return listWorkspaceProjects({
    fs: client.fs,
    workspaceMountPath: client.workspaceMountPath,
  });
}

export async function listWorkspaceProjects(
  context: WorkspaceProjectContext,
): Promise<SandboxProject[]> {
  try {
    const projectsStat = await context.fs.stat(SANDBOX_PROJECTS_DIR);
    if (!projectsStat.isDirectory) {
      throw new Error(
        `Path "${SANDBOX_PROJECTS_DIR}" already exists and is not a directory.`,
      );
    }
  } catch (error) {
    if (
      typeof error === "object" &&
      error !== null &&
      "code" in error &&
      (error as NodeJS.ErrnoException).code === "ENOENT"
    ) {
      return [];
    }
    throw error;
  }

  const entries = await context.fs.readdir(SANDBOX_PROJECTS_DIR);
  return entries
    .filter((entry) => entry.isDirectory)
    .flatMap((entry) => {
      try {
        const name = validateProjectName(entry.name);
        return [
          {
            name,
            relativePath: toProjectRelativePath(name),
            workspaceMountPath: toProjectMountPath(
              context.workspaceMountPath,
              name,
            ),
          },
        ];
      } catch {
        return [];
      }
    })
    .sort((a, b) => a.name.localeCompare(b.name));
}
