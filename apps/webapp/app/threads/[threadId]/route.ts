import { ThreadMemoryError } from "@protean/agent-memory";
import { requireUserId } from "@/lib/server/auth-user";
import { getAgentMemory } from "@/lib/server/agent-memory";
import {
  parseModelSelection,
  resolveModelSelection,
} from "@protean/model-catalog";
import { canAccessThread } from "@/lib/server/thread-utils";

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ threadId: string }> },
) {
  const userId = await requireUserId();

  if (!userId) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { threadId } = await params;
  const memory = await getAgentMemory(userId);
  const thread = await memory.getThreadWithMessages(threadId);
  if (!thread || !canAccessThread(thread, userId)) {
    return Response.json({ error: "Thread not found" }, { status: 404 });
  }

  return Response.json({ thread });
}

export async function DELETE(
  _request: Request,
  { params }: { params: Promise<{ threadId: string }> },
) {
  const userId = await requireUserId();

  if (!userId) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { threadId } = await params;
  const memory = await getAgentMemory(userId);
  const thread = await memory.getThread(threadId);
  if (!thread || !canAccessThread(thread, userId)) {
    return Response.json({ error: "Thread not found" }, { status: 404 });
  }
  await memory.softDeleteThread(threadId);

  return Response.json({ ok: true }, { status: 200 });
}

export async function PATCH(
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

  if (!body || typeof body !== "object") {
    return Response.json({ error: "Invalid request body" }, { status: 400 });
  }

  const hasOwn = (key: string) =>
    Object.prototype.hasOwnProperty.call(body, key);

  const parsedModelSelection = hasOwn("modelSelection")
    ? parseModelSelection(body.modelSelection)
    : undefined;
  if (hasOwn("modelSelection") && !parsedModelSelection) {
    return Response.json({ error: "Invalid request body" }, { status: 400 });
  }

  const projectName = hasOwn("projectName")
    ? typeof body.projectName === "string" ? body.projectName : undefined
    : undefined;
  if (hasOwn("projectName") && typeof body.projectName !== "string") {
    return Response.json({ error: "Invalid request body" }, { status: 400 });
  }

  if (!parsedModelSelection && !hasOwn("projectName")) {
    return Response.json({ error: "Invalid request body" }, { status: 400 });
  }

  const { threadId } = await params;
  const memory = await getAgentMemory(userId);
  const existing = await memory.getThread(threadId);
  if (!existing || !canAccessThread(existing, userId)) {
    return Response.json({ error: "Thread not found" }, { status: 404 });
  }

  let thread;
  try {
    thread = await memory.updateThreadSettings(threadId, {
      modelSelection: parsedModelSelection
        ? resolveModelSelection({ request: parsedModelSelection })
        : undefined,
      projectName,
    });
  } catch (error) {
    if (error instanceof ThreadMemoryError && error.code === "INVALID_STATE") {
      return Response.json({ error: error.message }, { status: 400 });
    }

    throw error;
  }

  if (!thread) {
    return Response.json({ error: "Thread not found" }, { status: 404 });
  }

  return Response.json({ thread }, { status: 200 });
}
