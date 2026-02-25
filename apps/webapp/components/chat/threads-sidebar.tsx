"use client";

import { useEffect } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  ArchiveIcon,
  MenuIcon,
  MoreHorizontalIcon,
  PlusIcon,
  StarIcon,
  Trash2Icon,
} from "lucide-react";
import type { ThreadRecordTrimmed } from "@protean/agent-memory";
import { useShallow } from "zustand/react/shallow";
import { formatCostUsd, formatTokenCount } from "@/lib/utils";
import { useThreadsSidebarActions } from "@/components/chat/actions/use-threads-sidebar-actions";
import { useRenderCountDebug } from "@/components/chat/utils/use-render-count-debug";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { SidebarUserMenu } from "@/components/chat/sidebar-user-menu";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { useThreadsSidebarStore } from "@/components/chat/state/threads-sidebar-store";

interface ThreadsSidebarProps {
  threads: ThreadRecordTrimmed[];
}

const serverTimestampFormatter = new Intl.DateTimeFormat("en-US", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "UTC",
});

function formatThreadUpdatedAt(value: string | Date, mounted: boolean): string {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return "";
  }

  if (mounted) {
    return date.toLocaleString();
  }

  return `${serverTimestampFormatter.format(date)} UTC`;
}

function sanitizeIdSegment(value: string): string {
  const sanitized = value.toLowerCase().replace(/[^a-z0-9_-]+/g, "-");
  return sanitized.length > 0 ? sanitized : "item";
}

export function ThreadsSidebar({ threads }: ThreadsSidebarProps) {
  useRenderCountDebug("ThreadsSidebar");

  const pathname = usePathname();
  const { deleteThreadItem, hydrateThreads, setMobileOpen, setMounted } =
    useThreadsSidebarActions();

  const { deletingThreadId, mobileOpen, mounted, threadItems } =
    useThreadsSidebarStore(
      useShallow((state) => ({
        deletingThreadId: state.deletingThreadId,
        mobileOpen: state.mobileOpen,
        mounted: state.mounted,
        threadItems: state.threadItems,
      })),
    );

  useEffect(() => {
    setMounted(true);
  }, [setMounted]);

  useEffect(() => {
    hydrateThreads(threads);
  }, [hydrateThreads, threads]);

  useEffect(() => {
    setMobileOpen(false);
  }, [pathname, setMobileOpen]);

  const renderSidebarContent = (menuScope: "desktop" | "mobile") => (
    <>
      <Button asChild className="w-full justify-start" size="sm">
        <Link href="/chats/new">
          <PlusIcon className="size-4" />
          New chat
        </Link>
      </Button>

      <ScrollArea className="mt-3 min-h-0 flex-1 pr-1">
        <div className="w-full space-y-1 pb-2">
          {threadItems.length === 0 ? (
            <p className="px-2 py-1 text-muted-foreground text-sm">
              No previous threads.
            </p>
          ) : (
            threadItems.map((thread) => {
              const isActive = pathname === `/chats/t/${thread.id}`;
              const isDeleting = deletingThreadId === thread.id;
              const idSegment = sanitizeIdSegment(thread.id);
              const triggerId = `thread-actions-${menuScope}-${idSegment}`;
              const contentId = `${triggerId}-content`;

              return (
                <div
                  className={`group flex min-w-0 items-start gap-1 rounded-md transition-colors ${
                    isActive ? "bg-accent" : "hover:bg-accent/70"
                  }`}
                  key={thread.id}
                >
                  <Link
                    className="block min-w-0 flex-1 overflow-hidden rounded-md px-2 py-2"
                    href={`/chats/t/${thread.id}`}
                  >
                    <p className="truncate font-medium text-sm">
                      {thread.title}
                    </p>
                    <p className="truncate text-muted-foreground text-xs">
                      {formatThreadUpdatedAt(thread.updatedAt, mounted)}
                    </p>
                    {thread.usage.inputTokens + thread.usage.outputTokens >
                    0 ? (
                      <p className="truncate text-muted-foreground text-xs">
                        {formatTokenCount(
                          thread.usage.inputTokens + thread.usage.outputTokens,
                        )}{" "}
                        tokens
                        {formatCostUsd(thread.usage.totalCostUsd)
                          ? ` · ${formatCostUsd(thread.usage.totalCostUsd)}`
                          : ""}
                      </p>
                    ) : null}
                  </Link>

                  <DropdownMenu>
                    <DropdownMenuTrigger asChild id={triggerId}>
                      <Button
                        className="mt-1.5 mr-1 h-7 w-7 shrink-0 p-0 text-muted-foreground opacity-80 hover:opacity-100"
                        disabled={isDeleting}
                        size="icon-sm"
                        type="button"
                        variant="ghost"
                      >
                        <MoreHorizontalIcon className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>

                    <DropdownMenuContent
                      align="end"
                      aria-labelledby={triggerId}
                      id={contentId}
                    >
                      <DropdownMenuItem
                        onSelect={(event) => {
                          event.preventDefault();
                          void deleteThreadItem(thread.id);
                        }}
                        variant="destructive"
                      >
                        <Trash2Icon className="size-4" />
                        Delete
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={(event) => {
                          event.preventDefault();
                        }}
                      >
                        <ArchiveIcon className="size-4" />
                        Archive
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onSelect={(event) => {
                          event.preventDefault();
                        }}
                      >
                        <StarIcon className="size-4" />
                        Star
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              );
            })
          )}
        </div>
      </ScrollArea>

      <div className="pt-3">
        <SidebarUserMenu />
      </div>
    </>
  );

  return (
    <>
      <aside className="hidden h-screen w-72 flex-col border-r border-sidebar-border bg-sidebar p-3 min-[1440px]:flex">
        {renderSidebarContent("desktop")}
      </aside>

      {mounted ? (
        <div className="fixed top-3 left-3 z-40 min-[1440px]:hidden">
          <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
            <SheetTrigger asChild>
              <Button size="icon" variant="outline">
                <MenuIcon className="size-5" />
                <span className="sr-only">Open sidebar</span>
              </Button>
            </SheetTrigger>
            <SheetContent
              side="left"
              showCloseButton={false}
              className="flex w-72 flex-col bg-sidebar p-3"
            >
              <SheetHeader className="p-0">
                <SheetTitle className="sr-only">Chat threads</SheetTitle>
              </SheetHeader>
              {renderSidebarContent("mobile")}
            </SheetContent>
          </Sheet>
        </div>
      ) : null}
    </>
  );
}
