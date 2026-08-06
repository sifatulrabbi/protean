import type {
  IProject,
  ISandboxClient,
  Mount,
  SandboxHealth,
  SandboxProject,
} from "@protean/contracts";
import {
  sandboxCopyResponseSchema,
  sandboxCreateResponseSchema,
  sandboxExecResponseSchema,
  sandboxHealthSchema,
  sandboxListDirResponseSchema,
  sandboxMoveResponseSchema,
  sandboxMutationResponseSchema,
  mountSchema,
  sandboxProjectSchema,
  sandboxReadFileResponseSchema,
  sandboxStatResponseSchema,
} from "@protean/contracts";

type FetchLike = (input: string | URL | Request, init?: RequestInit) => Promise<Response>;

type SandboxClientOptions = {
  baseUrl: string;
  token?: string;
  heartbeatIntervalMs?: number;
  fetch?: FetchLike;
};

type RequestOptions = {
  method?: "GET" | "POST" | "PUT" | "DELETE";
  body?: unknown;
};

type CreateMountPayload = {
  sourcePath: string;
  projectName: string;
};

const DEFAULT_HEARTBEAT_MS = 5_000;

function trimSlashes(value: string) {
  return value.replace(/^\/+|\/+$/g, "");
}

function toScopedPath(projectPath: string, relativeOrAbsolutePath: string) {
  const normalizedProjectPath = `/${trimSlashes(projectPath)}`;

  if (relativeOrAbsolutePath.startsWith(normalizedProjectPath)) {
    return relativeOrAbsolutePath;
  }

  return `${normalizedProjectPath}/${trimSlashes(relativeOrAbsolutePath)}`;
}

export class HttpSandboxClient implements ISandboxClient {
  public isConnected = false;
  public connErr: Error | null = null;

  private heartbeatTimer: Timer | null = null;
  private readonly fetchImpl: FetchLike;

  constructor(private readonly options: SandboxClientOptions) {
    this.fetchImpl = options.fetch ?? fetch;
  }

  private get headers() {
    return {
      "content-type": "application/json",
      ...(this.options.token ? { authorization: `Bearer ${this.options.token}` } : {}),
    };
  }

  private async request<T>(pathname: string, schema: { parse: (value: unknown) => T }, options?: RequestOptions) {
    const response = await this.fetchImpl(new URL(pathname, this.options.baseUrl), {
      method: options?.method ?? "GET",
      headers: this.headers,
      body: options?.body ? JSON.stringify(options.body) : undefined,
    });

    if (!response.ok) {
      throw new Error(`Sandbox request failed with ${response.status} at ${pathname}`);
    }

    const json = await response.json();
    return schema.parse(json);
  }

  private async ping() {
    try {
      await this.getHealth();
      this.isConnected = true;
      this.connErr = null;
    } catch (error) {
      this.isConnected = false;
      this.connErr = error instanceof Error ? error : new Error("Sandbox heartbeat failed.");
    }
  }

  startHeartbeat() {
    if (this.heartbeatTimer) {
      return;
    }

    void this.ping();
    this.heartbeatTimer = setInterval(() => {
      void this.ping();
    }, this.options.heartbeatIntervalMs ?? DEFAULT_HEARTBEAT_MS);
  }

  stopHeartbeat() {
    if (!this.heartbeatTimer) {
      return;
    }

    clearInterval(this.heartbeatTimer);
    this.heartbeatTimer = null;
  }

  async getHealth(): Promise<SandboxHealth> {
    return this.request("/health", sandboxHealthSchema);
  }

  async listProjects(): Promise<SandboxProject[]> {
    const projects = await this.request("/projects", {
      parse: (value: unknown) => {
        if (!Array.isArray(value)) {
          throw new Error("Expected project list.");
        }

        return value.map((entry) => sandboxProjectSchema.parse(entry));
      },
    });

    return projects;
  }

  async createMount(payload: CreateMountPayload): Promise<Mount> {
    return this.request("/mounts", mountSchema, {
      method: "POST",
      body: payload,
    });
  }

  bash = {
    exec: async (cmd: string, scope?: string) => {
      const response = await this.request("/bash/exec", sandboxExecResponseSchema, {
        method: "POST",
        body: { cmd, scope },
      });

      return {
        result: response.result,
        error: response.error ? new Error(response.error) : null,
      };
    },
  };

  fs = {
    readFile: async (fullPath: string, range = { startLine: 0 }) => {
      const response = await this.request("/fs/read-file", sandboxReadFileResponseSchema, {
        method: "POST",
        body: { fullPath, range },
      });

      return {
        ...response,
        error: response.error ? new Error(response.error) : null,
      };
    },
    listDir: async (fullPath: string) => {
      const response = await this.request("/fs/list-dir", sandboxListDirResponseSchema, {
        method: "POST",
        body: { fullPath },
      });

      return {
        ...response,
        error: response.error ? new Error(response.error) : null,
      };
    },
    stat: async (fullPath: string) => {
      const response = await this.request("/fs/stat", sandboxStatResponseSchema, {
        method: "POST",
        body: { fullPath },
      });

      return {
        ...response,
        error: response.error ? new Error(response.error) : null,
      };
    },
    writeFile: async (fullPath: string, content: string) => {
      const response = await this.request("/fs/write-file", sandboxMutationResponseSchema, {
        method: "POST",
        body: { fullPath, content },
      });

      return {
        ...response,
        error: response.error ? new Error(response.error) : null,
      };
    },
    create: async (fullPath: string, entityType: "file" | "directory", opts?: { recursive?: boolean }) => {
      const response = await this.request("/fs/create", sandboxCreateResponseSchema, {
        method: "POST",
        body: { fullPath, entityType, opts },
      });

      return {
        ...response,
        error: response.error ? new Error(response.error) : null,
      };
    },
    copy: async (sourceFullPath: string, copyToFullPath: string) => {
      const response = await this.request("/fs/copy", sandboxCopyResponseSchema, {
        method: "POST",
        body: { sourceFullPath, copyToFullPath },
      });

      return {
        ...response,
        error: response.error ? new Error(response.error) : null,
      };
    },
    move: async (sourceFullPath: string, newFullPath: string, opts?: { recursive?: boolean }) => {
      const response = await this.request("/fs/move", sandboxMoveResponseSchema, {
        method: "POST",
        body: { sourceFullPath, newFullPath, opts },
      });

      return {
        ...response,
        error: response.error ? new Error(response.error) : null,
      };
    },
    remove: async (fullPath: string, opts?: { recursive?: boolean }) => {
      const response = await this.request("/fs/remove", sandboxMutationResponseSchema, {
        method: "POST",
        body: { fullPath, opts },
      });

      return {
        ...response,
        error: response.error ? new Error(response.error) : null,
      };
    },
  };
}

export function createSandboxClient(options: SandboxClientOptions) {
  return new HttpSandboxClient(options);
}

export function createProject(payload: {
  sandboxClient: ISandboxClient;
  projectPath: string;
}): IProject {
  return {
    bash: {
      exec: (cmd, scope) =>
        payload.sandboxClient.bash.exec(cmd, scope ? toScopedPath(payload.projectPath, scope) : payload.projectPath),
    },
    fs: {
      readFile: (fullPath, range) => payload.sandboxClient.fs.readFile(toScopedPath(payload.projectPath, fullPath), range),
      listDir: (fullPath) => payload.sandboxClient.fs.listDir(toScopedPath(payload.projectPath, fullPath)),
      stat: (fullPath) => payload.sandboxClient.fs.stat(toScopedPath(payload.projectPath, fullPath)),
      writeFile: (fullPath, content) =>
        payload.sandboxClient.fs.writeFile(toScopedPath(payload.projectPath, fullPath), content),
      create: (fullPath, entityType, opts) =>
        payload.sandboxClient.fs.create(toScopedPath(payload.projectPath, fullPath), entityType, opts),
      copy: (sourceFullPath, copyToFullPath) =>
        payload.sandboxClient.fs.copy(
          toScopedPath(payload.projectPath, sourceFullPath),
          toScopedPath(payload.projectPath, copyToFullPath),
        ),
      move: (sourceFullPath, newFullPath, opts) =>
        payload.sandboxClient.fs.move(
          toScopedPath(payload.projectPath, sourceFullPath),
          toScopedPath(payload.projectPath, newFullPath),
          opts,
        ),
      remove: (fullPath, opts) => payload.sandboxClient.fs.remove(toScopedPath(payload.projectPath, fullPath), opts),
    },
  };
}

export { toScopedPath };
