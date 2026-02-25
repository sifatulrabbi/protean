"use client";

import { Fragment, type ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import {
  EyeIcon,
  DownloadIcon,
  MessageSquarePlusIcon,
  Trash2Icon,
  PencilIcon,
} from "lucide-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { isPreviewable } from "@/lib/file-utils";

// ─── Action definitions ──────────────────────────────────────────────────────

export interface FileEntryAction {
  id: string;
  label: string;
  icon: LucideIcon;
  disabled?: boolean;
  destructive?: boolean;
  onSelect: () => void;
}

export type FileEntryActionGroup = FileEntryAction[];

// ─── Props ───────────────────────────────────────────────────────────────────

export interface FileEntry {
  name: string;
  path: string;
  isDirectory: boolean;
  size: number;
  modified: string;
}

export interface FileEntryContextMenuProps {
  entry: FileEntry;
  children: ReactNode;
  onOpen: (entry: FileEntry) => void;
  onDownload: (entry: FileEntry) => void;
  onAddToChat: (entry: FileEntry) => void;
  onDelete: (entry: FileEntry) => void;
  onRename: (entry: FileEntry) => void;
}

// ─── Component ───────────────────────────────────────────────────────────────

export function FileEntryContextMenu({
  entry,
  children,
  onOpen,
  onDownload,
  onAddToChat,
  onDelete,
  onRename,
}: FileEntryContextMenuProps) {
  const ext = entry.name.split(".").pop()?.toLowerCase() ?? "";
  const canOpen = entry.isDirectory || isPreviewable(ext);

  // Actions are defined as groups separated by a divider.
  // To add a new action: add it to the appropriate group, or create a new group.
  const actionGroups: FileEntryActionGroup[] = [
    // Primary actions
    [
      {
        id: "open",
        label: "Open",
        icon: EyeIcon,
        disabled: !canOpen,
        onSelect: () => onOpen(entry),
      },
      {
        id: "download",
        label: "Download",
        icon: DownloadIcon,
        disabled: entry.isDirectory,
        onSelect: () => onDownload(entry),
      },
      {
        id: "add-to-chat",
        label: "Add to chat",
        icon: MessageSquarePlusIcon,
        onSelect: () => onAddToChat(entry),
      },
    ],
    // Mutation actions
    [
      {
        id: "rename",
        label: "Rename",
        icon: PencilIcon,
        onSelect: () => onRename(entry),
      },
      {
        id: "delete",
        label: "Delete",
        icon: Trash2Icon,
        destructive: true,
        onSelect: () => onDelete(entry),
      },
    ],
  ];

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-48">
        {actionGroups.map((group, groupIdx) => (
          <Fragment key={groupIdx}>
            {groupIdx > 0 && <ContextMenuSeparator />}
            {group.map((action) => {
              const Icon = action.icon;
              return (
                <ContextMenuItem
                  key={action.id}
                  disabled={action.disabled}
                  onSelect={action.onSelect}
                  className={
                    action.destructive
                      ? "text-destructive focus:text-destructive"
                      : ""
                  }
                >
                  <Icon className="mr-2 size-3.5" />
                  {action.label}
                </ContextMenuItem>
              );
            })}
          </Fragment>
        ))}
      </ContextMenuContent>
    </ContextMenu>
  );
}
