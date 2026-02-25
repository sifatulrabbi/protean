"use client";

import type React from "react";
import type { UIMessage } from "ai";
import type {
  ModelSelection,
  AIModelProviderEntry,
} from "@protean/model-catalog";
import type { ThreadUsage } from "@protean/agent-memory";
import { ModelCatalogProvider } from "@/components/chat/model-catalog-provider";
import { ThreadRuntimeController } from "@/components/chat/thread-runtime-controller";
import { ThreadErrorAlert } from "@/components/chat/thread-error-alert";
import { ThreadHeader } from "@/components/chat/thread-header";
import { ThreadMessages } from "@/components/chat/thread-messages";
import { ThreadPromptInput } from "@/components/chat/thread-user-input";
import { SidebarProvider, SidebarInset } from "@/components/ui/sidebar";
import { WorkspaceFilesPanel } from "@/components/chat/workspace-files-panel";

interface ThreadRouteContentProps {
  defaultModelSelection: ModelSelection;
  initialMessageUsageMap?: Record<string, ThreadUsage>;
  initialMessages?: UIMessage[];
  initialThreadId?: string;
  initialModelSelection?: ModelSelection;
  providers: AIModelProviderEntry[];
}

export function ThreadRouteContent({
  defaultModelSelection,
  initialMessageUsageMap,
  initialMessages,
  initialThreadId,
  initialModelSelection,
  providers,
}: ThreadRouteContentProps) {
  return (
    <ModelCatalogProvider providers={providers}>
      <ThreadRuntimeController
        defaultModelSelection={defaultModelSelection}
        initialMessageUsageMap={initialMessageUsageMap}
        initialMessages={initialMessages}
        initialModelSelection={initialModelSelection}
        initialThreadId={initialThreadId}
        providers={providers}
      />
      <SidebarProvider
        className="!min-h-0 h-full"
        defaultOpen={true}
        style={{ "--sidebar-width": "20rem" } as React.CSSProperties}
      >
        <SidebarInset>
          <div className="mx-auto flex min-h-0 w-full max-w-4xl flex-1 flex-col">
            <ThreadHeader />
            <ThreadMessages />
            <ThreadErrorAlert />
            <ThreadPromptInput />
          </div>
        </SidebarInset>
        <WorkspaceFilesPanel />
      </SidebarProvider>
    </ModelCatalogProvider>
  );
}
