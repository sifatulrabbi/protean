import type { UIMessage } from "ai";
import {
  parseModelSelection,
  resolveModelSelection,
} from "@protean/model-catalog";

import { requireUserId } from "@/lib/server/auth-user";
import { getAgentMemory } from "@/lib/server/agent-memory";
import { canAccessThread } from "@/lib/server/thread-utils";

export async function POST(
  request: Request,
  { params }: { params: Promise<{ threadId: string }> },
) {
  const [userId, body] = await Promise.all([
    requireUserId(),
    request.json().catch(() => null),
  ]);

  if (!userId) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (!body?.message) {
    return Response.json({ error: "Invalid request body" }, { status: 400 });
  }

  const { threadId } = await params;
  const memory = await getAgentMemory();
  let thread = await memory.getThreadWithMessages(threadId);

  if (!thread || !canAccessThread(thread, userId)) {
    return Response.json({ error: "Thread not found" }, { status: 404 });
  }

  const message = body.message as UIMessage;

  const resolvedModelSelection = resolveModelSelection({
    request: parseModelSelection(body.modelSelection),
    thread: thread.modelSelection,
  });

  const existingRecord = thread.history.find(
    (record) => record.deletedAt === null && record.message.id === message.id,
  );

  thread = await memory.upsertMessage(threadId, {
    message,
    modelSelection: resolvedModelSelection,
    ...(existingRecord
      ? {
          id: existingRecord.id,
          usage: existingRecord.usage,
        }
      : {
          usage: {
            inputTokens: 0,
            outputTokens: 0,
            totalDurationMs: 0,
          },
        }),
  });
  if (!thread || !canAccessThread(thread, userId)) {
    return Response.json({ error: "Thread not found" }, { status: 404 });
  }

  return Response.json({ thread }, { status: 200 });
}
