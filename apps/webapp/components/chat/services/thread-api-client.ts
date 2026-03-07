import type { UIMessage } from "ai";
import type { ModelSelection } from "@protean/model-catalog";

interface CreateThreadRequest {
  modelSelection: ModelSelection;
  initialUserMessage?: string;
  projectName?: string;
  title?: string;
}

interface CreateThreadResponse {
  thread: {
    id: string;
  };
}

export async function createThread({
  modelSelection,
  initialUserMessage,
  projectName,
  title,
}: CreateThreadRequest): Promise<string> {
  const response = await fetch("/threads", {
    body: JSON.stringify({
      initialUserMessage,
      modelSelection,
      projectName,
      title,
    }),
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  });

  if (!response.ok) {
    throw new Error("Unable to create chat thread");
  }

  const data = (await response.json()) as CreateThreadResponse;
  return data.thread.id;
}

export async function updateThreadSettings(args: {
  threadId: string;
  modelSelection?: ModelSelection;
  projectName?: string;
}): Promise<void> {
  const response = await fetch(`/threads/${args.threadId}`, {
    body: JSON.stringify({
      modelSelection: args.modelSelection,
      projectName: args.projectName,
    }),
    headers: {
      "Content-Type": "application/json",
    },
    method: "PATCH",
  });

  if (!response.ok) {
    throw new Error("Unable to update thread settings");
  }
}

export async function updateThreadModelSelection(args: {
  threadId: string;
  modelSelection: ModelSelection;
}): Promise<void> {
  await updateThreadSettings(args);
}

export async function deleteThread(threadId: string): Promise<boolean> {
  const response = await fetch(`/threads/${threadId}`, {
    method: "DELETE",
  });

  return response.ok;
}

export async function addMessage(args: {
  threadId: string;
  message: UIMessage;
  modelSelection?: ModelSelection;
}): Promise<void> {
  const response = await fetch(`/threads/${args.threadId}/messages`, {
    body: JSON.stringify({
      message: args.message,
      modelSelection: args.modelSelection,
    }),
    headers: {
      "Content-Type": "application/json",
    },
    method: "POST",
  });

  if (!response.ok) {
    throw new Error("Unable to add message");
  }
}

export async function editMessage(args: {
  threadId: string;
  messageId: string;
  message: UIMessage;
}): Promise<void> {
  const response = await fetch(
    `/threads/${args.threadId}/messages/${args.messageId}`,
    {
      body: JSON.stringify({ message: args.message }),
      headers: {
        "Content-Type": "application/json",
      },
      method: "PATCH",
    },
  );

  if (!response.ok) {
    throw new Error("Unable to edit message");
  }
}
