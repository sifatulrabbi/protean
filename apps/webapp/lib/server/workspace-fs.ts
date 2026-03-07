import {
  ensureSandboxSession,
} from "@protean/sandbox-client";
import { consoleLogger } from "@protean/logger";
import type { FS } from "@protean/vfs";

interface WorkspaceSandbox {
  fs: FS;
  sessionId: string;
  workspaceMountPath: string;
}

const pendingSandboxByUserId = new Map<string, Promise<WorkspaceSandbox>>();

function getSandboxConfig(): { baseUrl: string; serviceToken: string } {
  const baseUrl =
    process.env.SANDBOX_BASE_URL?.trim() ??
    process.env.SANDBOX_SERVICE_BASE_URL?.trim();
  const serviceToken = process.env.SANDBOX_SERVICE_TOKEN?.trim();

  if (!baseUrl) {
    throw new Error(
      "SANDBOX_BASE_URL (or SANDBOX_SERVICE_BASE_URL) is required.",
    );
  }

  if (!serviceToken) {
    throw new Error("SANDBOX_SERVICE_TOKEN is required.");
  }

  return { baseUrl, serviceToken };
}

async function createOrLoadSandbox(userId: string): Promise<WorkspaceSandbox> {
  const sandboxConfig = getSandboxConfig();
  const client = await ensureSandboxSession(
    {
      baseUrl: sandboxConfig.baseUrl,
      serviceToken: sandboxConfig.serviceToken,
      logger: consoleLogger,
    },
    {
      sessionId: userId,
    },
  );

  return {
    fs: client.fs,
    sessionId: client.sessionId,
    workspaceMountPath: client.workspaceMountPath,
  };
}

export async function getWorkspaceSandbox(userId: string): Promise<WorkspaceSandbox> {
  const pending = pendingSandboxByUserId.get(userId);
  if (pending) {
    return pending;
  }

  const nextPromise = createOrLoadSandbox(userId);
  pendingSandboxByUserId.set(userId, nextPromise);

  try {
    return await nextPromise;
  } catch (error) {
    pendingSandboxByUserId.delete(userId);
    throw error;
  }
}

export async function createWorkspaceFs(userId: string): Promise<FS> {
  const workspace = await getWorkspaceSandbox(userId);
  return workspace.fs;
}
