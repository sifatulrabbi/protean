"use client";

import { useCallback, useTransition } from "react";
import { usePathname, useRouter } from "next/navigation";
import type { ThreadRecordTrimmed } from "@protean/agent-memory";
import { deleteThread } from "@/components/chat/services/thread-api-client";
import { useThreadsSidebarStore } from "@/components/chat/state/threads-sidebar-store";

export function useThreadsSidebarActions() {
  const router = useRouter();
  const pathname = usePathname();
  const [, startTransition] = useTransition();

  const hydrateThreads = useCallback((threads: ThreadRecordTrimmed[]): void => {
    useThreadsSidebarStore.getState().hydrateThreads(threads);
  }, []);

  const setMounted = useCallback((mounted: boolean): void => {
    useThreadsSidebarStore.getState().setMounted(mounted);
  }, []);

  const setMobileOpen = useCallback((mobileOpen: boolean): void => {
    useThreadsSidebarStore.getState().setMobileOpen(mobileOpen);
  }, []);

  const deleteThreadItem = useCallback(
    async (threadId: string): Promise<void> => {
      const store = useThreadsSidebarStore.getState();
      store.setDeletingThreadId(threadId);

      try {
        const deleted = await deleteThread(threadId);
        if (!deleted) {
          return;
        }

        store.removeThread(threadId);

        startTransition(() => {
          if (pathname === `/chats/t/${threadId}`) {
            router.push("/chats/new");
          }

          router.refresh();
        });
      } finally {
        useThreadsSidebarStore.getState().setDeletingThreadId(null);
      }
    },
    [pathname, router],
  );

  return {
    deleteThreadItem,
    hydrateThreads,
    setMobileOpen,
    setMounted,
  };
}
