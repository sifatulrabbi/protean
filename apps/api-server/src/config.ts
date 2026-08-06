import path from "node:path";

export type ApiServerConfig = {
  host: string;
  port: number;
  apiBaseUrl: string;
  webappOrigin: string;
  dataDir: string;
  sandboxBaseUrl: string;
  sandboxToken?: string;
  defaultModelId: string;
  defaultInferenceProvider: string;
  repoRoot: string;
  skillsFilePath: string;
};

export function getApiServerConfig(): ApiServerConfig {
  const cwd = process.cwd();
  const repoRoot = path.resolve(cwd, "../..");
  const port = Number(process.env.API_PORT || 8788);

  return {
    host: process.env.API_HOST || "127.0.0.1",
    port,
    apiBaseUrl: process.env.API_BASE_URL || `http://127.0.0.1:${port}`,
    webappOrigin: process.env.WEBAPP_ORIGIN || "http://localhost:3000",
    dataDir:
      process.env.PROTEAN_DATA_DIR || path.join(repoRoot, "tmp", "app-data"),
    sandboxBaseUrl: process.env.SANDBOX_BASE_URL || "http://127.0.0.1:8787",
    sandboxToken: process.env.SANDBOX_SERVICE_TOKEN,
    defaultModelId: process.env.PROTEAN_DEFAULT_MODEL_ID || "gpt-4o-mini",
    defaultInferenceProvider:
      process.env.PROTEAN_DEFAULT_INFERENCE_PROVIDER || "openrouter",
    repoRoot,
    skillsFilePath: path.join(repoRoot, "AGENTS.md"),
  };
}
