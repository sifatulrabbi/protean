"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  CloudIcon,
  HardDriveIcon,
  PlusIcon,
  RefreshCwIcon,
} from "lucide-react";

import { useWorkspaceFilesActions } from "@/components/chat/actions/use-workspace-files-actions";
import {
  FileTree,
  FileTreeFile,
  FileTreeFolder,
} from "@/components/ai-elements/file-tree";
import {
  FileEntryContextMenu,
  type FileEntry,
} from "@/components/chat/file-entry-context-menu";
import { FileViewerDialog } from "@/components/chat/file-viewer-dialog";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
} from "@/components/ui/sidebar";
import { useWorkspaceFilesStore } from "@/components/chat/state/workspace-files-store";
import type {
  StorageType,
  WorkspaceProjectTreeNode,
} from "@/components/chat/state/types";

function nodeToFileEntry(node: WorkspaceProjectTreeNode): FileEntry {
  return {
    name: node.name,
    path: node.workspacePath,
    isDirectory: node.isDirectory,
    size: node.size ?? 0,
    modified: node.modified ?? "",
  };
}

interface FileTreeNodesProps {
  nodes: WorkspaceProjectTreeNode[];
  onOpen: (entry: FileEntry) => void;
  onDownload: (entry: FileEntry) => void;
  onAddToChat: (entry: FileEntry) => void;
  onDelete: (entry: FileEntry) => void;
  onRename: (entry: FileEntry) => void;
}

function FileTreeNodes({
  nodes,
  onOpen,
  onDownload,
  onAddToChat,
  onDelete,
  onRename,
}: FileTreeNodesProps) {
  return nodes.map((node) => {
    const entry = nodeToFileEntry(node);

    if (node.isDirectory) {
      return (
        <FileEntryContextMenu
          key={node.workspacePath}
          entry={entry}
          onOpen={onOpen}
          onDownload={onDownload}
          onAddToChat={onAddToChat}
          onDelete={onDelete}
          onRename={onRename}
        >
          <div>
            <FileTreeFolder path={node.workspacePath} name={node.name}>
              {node.children?.length ? (
                <FileTreeNodes
                  nodes={node.children}
                  onOpen={onOpen}
                  onDownload={onDownload}
                  onAddToChat={onAddToChat}
                  onDelete={onDelete}
                  onRename={onRename}
                />
              ) : null}
            </FileTreeFolder>
          </div>
        </FileEntryContextMenu>
      );
    }

    return (
      <FileEntryContextMenu
        key={node.workspacePath}
        entry={entry}
        onOpen={onOpen}
        onDownload={onDownload}
        onAddToChat={onAddToChat}
        onDelete={onDelete}
        onRename={onRename}
      >
        <div>
          <FileTreeFile path={node.workspacePath} name={node.name} />
        </div>
      </FileEntryContextMenu>
    );
  });
}

function flattenNodes(
  nodes: WorkspaceProjectTreeNode[],
  map: Map<string, WorkspaceProjectTreeNode>,
) {
  for (const node of nodes) {
    map.set(node.workspacePath, node);
    if (node.children?.length) {
      flattenNodes(node.children, map);
    }
  }
}

export function WorkspaceFilesPanel() {
  const {
    createProject,
    deleteFile,
    refreshProjectTree,
    refreshProjects,
    refreshSkills,
    renameFile,
    selectProject,
  } = useWorkspaceFilesActions();

  const storageType = useWorkspaceFilesStore((state) => state.storageType);
  const projects = useWorkspaceFilesStore((state) => state.projects);
  const selectedProjectName = useWorkspaceFilesStore(
    (state) => state.selectedProjectName,
  );
  const projectsLoading = useWorkspaceFilesStore(
    (state) => state.projectsLoading,
  );
  const projectsError = useWorkspaceFilesStore((state) => state.projectsError);
  const projectTree = useWorkspaceFilesStore((state) => state.projectTree);
  const projectTreeError = useWorkspaceFilesStore(
    (state) => state.projectTreeError,
  );
  const projectTreeLoading = useWorkspaceFilesStore(
    (state) => state.projectTreeLoading,
  );
  const selectedFileWorkspacePath = useWorkspaceFilesStore(
    (state) => state.selectedFileWorkspacePath,
  );
  const skills = useWorkspaceFilesStore((state) => state.skills);
  const skillsLoading = useWorkspaceFilesStore((state) => state.skillsLoading);
  const skillsError = useWorkspaceFilesStore((state) => state.skillsError);
  const setSelectedFileWorkspacePath = useWorkspaceFilesStore(
    (state) => state.setSelectedFileWorkspacePath,
  );
  const setStorageType = useWorkspaceFilesStore(
    (state) => state.setStorageType,
  );

  const [createOpen, setCreateOpen] = useState(false);
  const [viewerPath, setViewerPath] = useState<string | null>(null);
  const [viewerName, setViewerName] = useState<string>("");
  const [projectNameInput, setProjectNameInput] = useState("");
  const [creatingProject, setCreatingProject] = useState(false);

  // Delete confirmation state
  const [deleteTarget, setDeleteTarget] = useState<FileEntry | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Rename state
  const [renameTarget, setRenameTarget] = useState<FileEntry | null>(null);
  const [renameInput, setRenameInput] = useState("");
  const [renaming, setRenaming] = useState(false);

  const hasSelectedProject = projects.some(
    (project) => project.name === selectedProjectName,
  );
  const nodeByWorkspacePath = useMemo(() => {
    const map = new Map<string, WorkspaceProjectTreeNode>();
    flattenNodes(projectTree, map);
    return map;
  }, [projectTree]);

  useEffect(() => {
    if (storageType !== "cloud") {
      return;
    }

    void (async () => {
      await refreshProjects();
      await refreshProjectTree();
      await refreshSkills();
    })();
  }, [refreshProjectTree, refreshProjects, refreshSkills, storageType]);

  useEffect(() => {
    if (storageType !== "cloud") {
      return;
    }

    void refreshProjectTree(selectedProjectName);
  }, [refreshProjectTree, selectedProjectName, storageType]);

  async function handleCreateProject(): Promise<void> {
    const trimmedName = projectNameInput.trim();
    if (!trimmedName) {
      return;
    }

    setCreatingProject(true);
    const didCreate = await createProject(trimmedName);
    setCreatingProject(false);

    if (didCreate) {
      setProjectNameInput("");
      setCreateOpen(false);
    }
  }

  async function handleDeleteConfirm(): Promise<void> {
    if (!deleteTarget) return;
    setDeleting(true);
    await deleteFile(deleteTarget.path);
    setDeleting(false);
    setDeleteTarget(null);
  }

  async function handleRenameConfirm(): Promise<void> {
    if (!renameTarget) return;
    const trimmed = renameInput.trim();
    if (!trimmed || trimmed === renameTarget.name) {
      setRenameTarget(null);
      return;
    }
    setRenaming(true);
    await renameFile(renameTarget.path, trimmed);
    setRenaming(false);
    setRenameTarget(null);
  }

  // Context menu handlers
  const handleContextOpen = useCallback(
    (entry: FileEntry) => {
      if (entry.isDirectory) return;
      setViewerName(entry.name);
      setViewerPath(entry.path);
    },
    [],
  );

  const handleContextDownload = useCallback((entry: FileEntry) => {
    if (entry.isDirectory) return;
    const link = document.createElement("a");
    link.href = `/api/files/${encodeURIComponent(entry.path)}`;
    link.download = entry.name;
    link.click();
  }, []);

  const handleContextAddToChat = useCallback((_entry: FileEntry) => {
    // TODO: integrate with chat input
  }, []);

  const handleContextDelete = useCallback((entry: FileEntry) => {
    setDeleteTarget(entry);
  }, []);

  const handleContextRename = useCallback((entry: FileEntry) => {
    setRenameTarget(entry);
    setRenameInput(entry.name);
  }, []);

  return (
    <>
      <Sidebar side="right" collapsible="offcanvas">
        <SidebarContent className="h-full">
          <div className="flex h-full flex-col">
            {/* Workspace section — takes remaining space */}
            <SidebarGroup className="flex min-h-0 flex-1 flex-col border-b py-3">
              <SidebarGroupLabel className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Workspace
              </SidebarGroupLabel>
              <SidebarGroupContent className="flex min-h-0 flex-1 flex-col gap-3 px-2">
                {/* Storage & Project selectors row */}
                <div className="flex items-center gap-2">
                  <Select
                    value={storageType}
                    onValueChange={(value: StorageType) =>
                      setStorageType(value)
                    }
                  >
                    <SelectTrigger size="sm" className="w-[130px] shrink-0">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent position="popper" align="start">
                      <SelectItem value="cloud">
                        <CloudIcon className="size-3.5" />
                        Cloud
                      </SelectItem>
                      <SelectItem value="local">
                        <HardDriveIcon className="size-3.5" />
                        Local
                      </SelectItem>
                    </SelectContent>
                  </Select>

                  <span className="text-muted-foreground/40 text-xs">/</span>

                  <Select
                    value={
                      hasSelectedProject ? selectedProjectName : undefined
                    }
                    onValueChange={(value) => {
                      void selectProject(value);
                    }}
                    disabled={storageType !== "cloud" || projectsLoading}
                  >
                    <SelectTrigger size="sm" className="min-w-0 flex-1">
                      <SelectValue placeholder="Project" />
                    </SelectTrigger>
                    <SelectContent position="popper" align="start">
                      {projects.length > 0 ? (
                        projects.map((project) => (
                          <SelectItem key={project.name} value={project.name}>
                            {project.name}
                          </SelectItem>
                        ))
                      ) : (
                        <SelectItem disabled value="__no_projects__">
                          No projects
                        </SelectItem>
                      )}
                    </SelectContent>
                  </Select>

                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-7 shrink-0 text-muted-foreground hover:text-foreground"
                    onClick={() => {
                      void refreshProjectTree(selectedProjectName);
                      void refreshSkills();
                    }}
                    disabled={
                      storageType !== "cloud" ||
                      !hasSelectedProject ||
                      projectTreeLoading
                    }
                  >
                    <RefreshCwIcon
                      className={`size-3.5 ${projectTreeLoading ? "animate-spin" : ""}`}
                    />
                    <span className="sr-only">Refresh project files</span>
                  </Button>

                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    className="size-7 shrink-0 text-muted-foreground hover:text-foreground"
                    onClick={() => setCreateOpen(true)}
                    disabled={storageType !== "cloud"}
                  >
                    <PlusIcon className="size-3.5" />
                    <span className="sr-only">Create project</span>
                  </Button>
                </div>

                {projectsLoading ? (
                  <p className="text-muted-foreground px-1 text-xs">
                    Loading projects...
                  </p>
                ) : null}

                {projectsError ? (
                  <p className="text-destructive px-1 text-xs">
                    {projectsError}
                  </p>
                ) : null}

                {/* Files area */}
                <div className="flex min-h-0 flex-1 flex-col">
                  <p className="mb-1.5 px-1 text-xs font-medium text-muted-foreground">
                    Files
                  </p>
                  {projectTreeLoading ? (
                    <p className="text-muted-foreground px-1 text-xs">
                      Loading files...
                    </p>
                  ) : null}
                  {projectTreeError ? (
                    <p className="text-destructive px-1 text-xs">
                      {projectTreeError}
                    </p>
                  ) : null}
                  {!projectTreeLoading && projectTree.length === 0 ? (
                    <div className="flex flex-1 items-center justify-center rounded-lg border border-dashed p-4">
                      <p className="text-center text-xs text-muted-foreground">
                        No files yet.
                        <br />
                        <span className="text-muted-foreground/60">
                          Files will appear here once added to the project.
                        </span>
                      </p>
                    </div>
                  ) : null}
                  {projectTree.length > 0 ? (
                    <ScrollArea className="min-h-0 flex-1">
                      <FileTree
                        className="border-none bg-transparent shadow-none"
                        selectedPath={selectedFileWorkspacePath ?? undefined}
                        onSelect={(value) => {
                          const workspacePath =
                            typeof value === "string" ? value : "";
                          if (!workspacePath) {
                            return;
                          }

                          const node = nodeByWorkspacePath.get(workspacePath);
                          if (!node || node.isDirectory) {
                            setSelectedFileWorkspacePath(null);
                            return;
                          }

                          setSelectedFileWorkspacePath(workspacePath);
                          setViewerName(node.name);
                          setViewerPath(workspacePath);
                        }}
                      >
                        <FileTreeNodes
                          nodes={projectTree}
                          onOpen={handleContextOpen}
                          onDownload={handleContextDownload}
                          onAddToChat={handleContextAddToChat}
                          onDelete={handleContextDelete}
                          onRename={handleContextRename}
                        />
                      </FileTree>
                    </ScrollArea>
                  ) : null}
                </div>

                {storageType === "local" ? (
                  <p className="text-muted-foreground px-1 text-xs">
                    Local workspace mode is not available yet.
                  </p>
                ) : null}
              </SidebarGroupContent>
            </SidebarGroup>

            {/* Skills section — max 30% of panel height */}
            <SidebarGroup className="min-h-0 max-h-[30%] shrink-0 py-3">
              <SidebarGroupLabel className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Skills
              </SidebarGroupLabel>
              <SidebarGroupContent className="min-h-0 px-2">
                {skillsLoading ? (
                  <p className="text-muted-foreground px-1 py-2 text-xs">
                    Loading skills...
                  </p>
                ) : null}

                {skillsError ? (
                  <p className="text-destructive px-1 py-2 text-xs">
                    {skillsError}
                  </p>
                ) : null}

                {!skillsLoading && skills.length === 0 ? (
                  <p className="text-muted-foreground/60 px-1 py-2 text-xs">
                    No skills found in workspace.
                  </p>
                ) : null}

                {skills.length > 0 ? (
                  <TooltipProvider delayDuration={300}>
                    <ScrollArea className="h-full pr-1">
                      <div className="flex flex-wrap gap-1.5 pb-2">
                        {skills.map((skill) => (
                          <Tooltip key={skill.path}>
                            <TooltipTrigger asChild>
                              <span className="inline-flex cursor-default rounded-md border bg-background/60 px-2 py-1 text-xs font-medium">
                                {skill.name}
                              </span>
                            </TooltipTrigger>
                            {skill.description ? (
                              <TooltipContent
                                side="left"
                                className="max-w-[240px] text-xs"
                              >
                                {skill.description}
                              </TooltipContent>
                            ) : null}
                          </Tooltip>
                        ))}
                      </div>
                    </ScrollArea>
                  </TooltipProvider>
                ) : null}
              </SidebarGroupContent>
            </SidebarGroup>
          </div>
        </SidebarContent>
      </Sidebar>

      {/* Create project dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Create project</DialogTitle>
          </DialogHeader>

          <Input
            value={projectNameInput}
            onChange={(event) => setProjectNameInput(event.target.value)}
            placeholder="Project name"
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                void handleCreateProject();
              }
            }}
          />

          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setCreateOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="button"
              disabled={creatingProject || projectNameInput.trim().length === 0}
              onClick={() => {
                void handleCreateProject();
              }}
            >
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirmation dialog */}
      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
      >
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete {deleteTarget?.isDirectory ? "folder" : "file"}</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete{" "}
              <span className="font-medium text-foreground">
                {deleteTarget?.name}
              </span>
              ? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setDeleteTarget(null)}
              disabled={deleting}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={deleting}
              onClick={() => {
                void handleDeleteConfirm();
              }}
            >
              {deleting ? "Deleting..." : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Rename dialog */}
      <Dialog
        open={renameTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRenameTarget(null);
        }}
      >
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Rename {renameTarget?.isDirectory ? "folder" : "file"}</DialogTitle>
          </DialogHeader>
          <Input
            value={renameInput}
            onChange={(event) => setRenameInput(event.target.value)}
            placeholder="New name"
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                void handleRenameConfirm();
              }
            }}
          />
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setRenameTarget(null)}
              disabled={renaming}
            >
              Cancel
            </Button>
            <Button
              type="button"
              disabled={
                renaming ||
                renameInput.trim().length === 0 ||
                renameInput.trim() === renameTarget?.name
              }
              onClick={() => {
                void handleRenameConfirm();
              }}
            >
              {renaming ? "Renaming..." : "Rename"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <FileViewerDialog
        fileName={viewerName}
        filePath={viewerPath ?? ""}
        onOpenChange={(open) => {
          if (!open) {
            setViewerPath(null);
          }
        }}
        open={viewerPath !== null}
      />
    </>
  );
}
