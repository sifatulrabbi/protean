import { createFsMemory, type AgentMemory } from "@protean/agent-memory";
import { consoleLogger } from "@protean/logger";
import { createWorkspaceFs } from "@/lib/server/workspace-fs";

const memoryByUserId = new Map<string, Promise<AgentMemory>>();

export async function getAgentMemory(userId: string): Promise<AgentMemory> {
  const pending = memoryByUserId.get(userId);
  if (pending) {
    return pending;
  }

  const nextPromise = (async () => {
    const fs = await createWorkspaceFs(userId);
    return createFsMemory({ fs, dirPath: ".threads" }, consoleLogger);
  })();

  memoryByUserId.set(userId, nextPromise);

  try {
    return await nextPromise;
  } catch (error) {
    memoryByUserId.delete(userId);
    throw error;
  }
}
