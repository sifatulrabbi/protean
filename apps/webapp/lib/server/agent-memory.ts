import { createFsMemory, type AgentMemory } from "@protean/agent-memory";
import { consoleLogger } from "@protean/logger";
import { createRemoteFs } from "@protean/vfs";

let memoryPromise: AgentMemory | null = null;

export async function getAgentMemory(): Promise<AgentMemory> {
  if (!memoryPromise) {
    const fs = await createRemoteFs({
      baseUrl: process.env.VFS_SERVER_URL!,
      serviceToken: process.env.VFS_SERVICE_TOKEN!,
      userId: "protean-memory",
      logger: consoleLogger,
    });
    memoryPromise = await createFsMemory({ fs }, consoleLogger);
  }

  return memoryPromise;
}
