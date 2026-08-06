import path from "node:path";

export type SandboxServerConfig = {
  host: string;
  port: number;
  workspaceRoot: string;
  virtualWorkspaceRoot: string;
  projectsRoot: string;
  mountsRoot: string;
  serviceToken?: string;
};

export function getSandboxServerConfig(): SandboxServerConfig {
  const workspaceRoot =
    process.env.PROTEAN_WORKSPACE_ROOT ||
    path.join(process.cwd(), "tmp", "workspace");
  const virtualWorkspaceRoot = "/workspace";

  return {
    host: process.env.SANDBOX_HOST || "127.0.0.1",
    port: Number(process.env.SANDBOX_PORT || 8787),
    workspaceRoot,
    virtualWorkspaceRoot,
    projectsRoot: path.join(workspaceRoot, "projects"),
    mountsRoot: path.join(workspaceRoot, "mounts"),
    serviceToken: process.env.SANDBOX_SERVICE_TOKEN,
  };
}
