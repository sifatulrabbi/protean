import { redirect } from "next/navigation";
import type { ThreadRecord } from "@protean/agent-memory";

import { auth } from "@/auth";
import { ThreadRouteContent } from "@/components/chat/thread-route-content";
import { getAgentMemory } from "@/lib/server/agent-memory";
import {
  getDefaultModelSelection,
  getModelCatalog,
  resolveModelSelection,
} from "@protean/model-catalog";
import {
  canAccessThread,
  threadToMessageUsageMap,
  threadToUiMessages,
} from "@/lib/server/thread-utils";

export default async function ThreadPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const session = await auth();

  if (!session?.user?.email) {
    redirect("/login");
  }

  const memory = await getAgentMemory();
  let thread: ThreadRecord | null = null;
  try {
    thread = await memory.getThreadWithMessages(id);
  } catch {}
  if (!thread || !canAccessThread(thread, session.user.email)) {
    redirect("/chats/new");
  }

  const providers = getModelCatalog();
  const defaultModelSelection = getDefaultModelSelection();
  const initialModelSelection = resolveModelSelection({
    thread: thread.modelSelection,
  });

  return (
    <ThreadRouteContent
      defaultModelSelection={defaultModelSelection}
      initialMessageUsageMap={threadToMessageUsageMap(thread)}
      initialMessages={threadToUiMessages(thread)}
      initialModelSelection={initialModelSelection}
      initialThreadId={thread.id}
      providers={providers}
    />
  );
}
