import { convertToModelMessages, type UIMessage } from "ai";
import { createRootAgent } from "@protean/protean";
import {
  deriveActiveHistory,
  type ThreadMessageRecord,
} from "@protean/agent-memory";
import { consoleLogger } from "@protean/logger";
import { createRemoteFs } from "@protean/vfs";
import {
  findModel,
  isSameModelSelection,
  parseModelSelection,
  resolveModelSelection,
} from "@protean/model-catalog";

import { requireUserId } from "@/lib/server/auth-user";
import { getAgentMemory } from "@/lib/server/agent-memory";
import { canAccessThread } from "@/lib/server/thread-utils";

function isPendingMessage(message: UIMessage): boolean {
  const metadata = (
    message as UIMessage & {
      metadata?: unknown;
    }
  ).metadata;

  if (!metadata || typeof metadata !== "object") {
    return false;
  }

  return (metadata as Record<string, unknown>).pending === true;
}

function clearPendingFlag(message: UIMessage): UIMessage {
  const metadata = (
    message as UIMessage & {
      metadata?: unknown;
    }
  ).metadata;

  if (!metadata || typeof metadata !== "object") {
    return message;
  }

  const nextMetadata = { ...(metadata as Record<string, unknown>) };
  delete nextMetadata.pending;

  return {
    ...message,
    metadata: nextMetadata,
  } as UIMessage;
}

function summarizeHistory(history: ThreadMessageRecord[]): UIMessage {
  const summaryText = history
    .map((item) => {
      const parts = item.message.parts as Array<{
        type: string;
        text?: string;
      }>;

      return parts
        .filter((part) => part.type === "text")
        .map((part) => part.text ?? "")
        .join(" ")
        .trim();
    })
    .filter((text) => text.length > 0)
    .join("\n")
    .slice(-4000);

  return {
    id: `summary-${Date.now()}`,
    role: "user",
    parts: [
      {
        type: "text",
        text: summaryText || "Conversation summary.",
      },
    ],
  };
}

export async function POST(request: Request) {
  const [userId, body] = await Promise.all([
    requireUserId(),
    request.json().catch(() => null),
  ]);

  if (!userId) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }

  const parsedBody = body as {
    invokePending?: boolean;
    modelSelection?: unknown;
    threadId?: string;
  } | null;

  if (!parsedBody?.threadId) {
    return Response.json({ error: "Invalid request body" }, { status: 400 });
  }
  const threadId = parsedBody.threadId;
  const memory = await getAgentMemory();

  let thread = await memory.getThreadWithMessages(threadId);
  if (!thread || !canAccessThread(thread, userId)) {
    return Response.json({ error: "Thread not found" }, { status: 404 });
  }

  const fs = await createRemoteFs({
    baseUrl: process.env.VFS_SERVER_URL!,
    serviceToken: process.env.VFS_SERVICE_TOKEN!,
    userId,
    logger: consoleLogger,
  });

  // For making sure the model selection and the reasoning budget are valid.
  const requestSelection = parseModelSelection(parsedBody.modelSelection);
  const resolvedModelSelection = resolveModelSelection({
    request: requestSelection,
    thread: thread.modelSelection,
  });

  if (!isSameModelSelection(thread.modelSelection, resolvedModelSelection)) {
    thread = await memory.updateThreadSettings(threadId, {
      modelSelection: resolvedModelSelection,
    });
    if (!thread || !canAccessThread(thread, userId)) {
      return Response.json({ error: "Thread not found" }, { status: 404 });
    }
  }

  /** The full model entry from openrouter */
  const fullModelEntry = findModel(
    resolvedModelSelection.providerId,
    resolvedModelSelection.modelId,
  );

  if (!fullModelEntry) {
    return Response.json(
      { error: "No valid model configuration available." },
      { status: 500 },
    );
  }

  if (parsedBody.invokePending) {
    const pendingIndex = [...thread.history]
      .map((tmsg, index) => ({ index, tmsg }))
      .reverse()
      .find(
        ({ tmsg }) =>
          tmsg.message.role === "user" && isPendingMessage(tmsg.message),
      )?.index;

    if (pendingIndex === undefined) {
      return Response.json(
        { error: "No pending thread message found" },
        { status: 409 },
      );
    }

    const pendingTmsg = thread.history[pendingIndex];
    pendingTmsg.message = clearPendingFlag(
      thread.history[pendingIndex].message,
    );

    thread = await memory.upsertMessage(threadId, {
      id: pendingTmsg.id,
      message: pendingTmsg.message,
      modelSelection: thread.modelSelection,
      usage: pendingTmsg.usage,
    });
    if (!thread || !canAccessThread(thread, userId)) {
      return Response.json({ error: "Thread not found" }, { status: 404 });
    }

    // meaning that we already have an assistant message for the user's message and
    // it's likely a state issue for thinking this is a pending message.
    if (thread.history[pendingIndex + 1]?.message.role === "assistant") {
      return Response.json(
        { error: "No pending thread message found" },
        { status: 409 },
      );
    }
  }

  const agent = await createRootAgent(
    {
      fs,
      modelSelection: {
        providerId: fullModelEntry.providerId,
        modelId: resolvedModelSelection.modelId,
        reasoningBudget: resolvedModelSelection.reasoningBudget,
        runtimeProvider: fullModelEntry.runtimeProvider,
      },
    },
    consoleLogger,
  );

  const compactionResult = await memory.compactIfNeeded(threadId, {
    policy: {
      maxContextTokens: fullModelEntry.contextLimits.total,
      reservedOutputTokens: fullModelEntry.contextLimits.maxOutput,
    },
    summarizeHistory: async (history) => summarizeHistory(history),
  });

  if (compactionResult) {
    thread = compactionResult.thread;
    if (!thread) {
      return Response.json({ error: "Thread not found" }, { status: 404 });
    }
  }

  const activeHistory = deriveActiveHistory(thread).map(
    (record) => record.message,
  );

  const streamStartMs = Date.now();
  const stream = await agent.stream({
    messages: await convertToModelMessages(activeHistory),
    abortSignal: request.signal,
  });

  return stream.toUIMessageStreamResponse({
    sendReasoning: true,
    sendSources: false,
    originalMessages: activeHistory,
    onFinish: async ({ isAborted, responseMessage }) => {
      const reloaded = await memory.getThreadWithMessages(threadId);
      if (!reloaded || !canAccessThread(reloaded, userId)) {
        return;
      }

      const totalDurationMs = Math.max(Date.now() - streamStartMs, 0);
      let inputTokens = 0;
      let outputTokens = 0;

      try {
        if (!isAborted) {
          const usage = await stream.usage;
          inputTokens = usage.inputTokens ?? 0;
          outputTokens = usage.outputTokens ?? 0;
        }
      } catch {
        inputTokens = 0;
        outputTokens = 0;
      }

      await memory.upsertMessage(threadId, {
        message: responseMessage,
        modelSelection: thread.modelSelection,
        usage: {
          inputTokens,
          outputTokens,
          totalDurationMs,
        },
      });
    },
    onError: (error) => {
      console.error("Failed to finish agent run:", error);

      if (error instanceof Error) {
        return error.message;
      }
      return "Failed to stream response from model.";
    },
  });
}
