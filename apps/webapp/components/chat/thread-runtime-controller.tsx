"use client";

import type { UIMessage } from "ai";
import type {
  ModelSelection,
  AIModelProviderEntry,
} from "@protean/model-catalog";
import type { ThreadUsage } from "@protean/agent-memory";
import { useThreadRuntime } from "@/components/chat/actions/use-thread-runtime";

interface ThreadRuntimeControllerProps {
  defaultModelSelection: ModelSelection;
  initialMessageUsageMap?: Record<string, ThreadUsage>;
  initialMessages?: UIMessage[];
  initialModelSelection?: ModelSelection;
  initialThreadId?: string;
  providers: AIModelProviderEntry[];
}

export function ThreadRuntimeController({
  defaultModelSelection,
  initialMessageUsageMap,
  initialMessages = [],
  initialModelSelection,
  initialThreadId,
  providers,
}: ThreadRuntimeControllerProps) {
  useThreadRuntime({
    defaultModelSelection,
    initialMessageUsageMap,
    initialMessages,
    initialModelSelection,
    initialThreadId,
    providers,
  });

  return null;
}
