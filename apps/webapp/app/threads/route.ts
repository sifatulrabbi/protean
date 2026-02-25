import { randomUUID } from "node:crypto";
import {
  resolveModelSelection,
  parseModelSelection,
} from "@protean/model-catalog";
import type { ThreadRecord } from "@protean/agent-memory";

import { requireUserId } from "@/lib/server/auth-user";
import { getAgentMemory } from "@/lib/server/agent-memory";

export async function GET() {
  const userId = await requireUserId();

  if (!userId) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const memory = await getAgentMemory();
  return Response.json(
    { threads: await memory.listThreads({ userId }) },
    { status: 200 },
  );
}

export async function POST(request: Request) {
  const [userId, body] = await Promise.all([
    requireUserId(),
    request.json().catch(() => ({})),
  ]);

  if (!userId) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const title = typeof body?.title === "string" ? body.title : undefined;
  const initialUserMessage =
    typeof body?.initialUserMessage === "string" &&
    body.initialUserMessage.trim().length > 0
      ? body.initialUserMessage.trim()
      : undefined;

  const modelSelection = resolveModelSelection({
    request: parseModelSelection(body?.modelSelection),
  });

  const memory = await getAgentMemory();
  let thread: ThreadRecord | null = await memory.createThread({
    userId,
    title: title?.trim() || "New chat",
    modelSelection,
  });

  if (!thread) {
    return Response.json(
      { message: "Failed to start a new thread" },
      { status: 400 },
    );
  }

  if (initialUserMessage) {
    thread = await memory.upsertMessage(thread.id, {
      message: {
        id: randomUUID(),
        metadata: { pending: true },
        parts: [{ text: initialUserMessage, type: "text" }],
        role: "user",
      },
      modelSelection,
      usage: {
        inputTokens: 0,
        outputTokens: 0,
        totalDurationMs: 0,
        totalCostUsd: 0,
      },
    });
  }

  return Response.json({ thread }, { status: 201 });
}
