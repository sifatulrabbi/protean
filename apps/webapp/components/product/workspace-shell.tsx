"use client";

import { startTransition, useEffect, useMemo, useState } from "react";

import {
  Conversation,
  ConversationContent,
  ConversationEmptyState,
  ConversationScrollButton,
} from "@/components/ai-elements/conversation";
import {
  FileTree,
  FileTreeFile,
  FileTreeFolder,
} from "@/components/ai-elements/file-tree";
import {
  Message,
  MessageContent,
  MessageResponse,
} from "@/components/ai-elements/message";
import {
  PromptInput,
  PromptInputBody,
  PromptInputFooter,
  PromptInputSubmit,
  PromptInputTextarea,
  PromptInputTools,
} from "@/components/ai-elements/prompt-input";
import { Task, TaskContent, TaskItem, TaskTrigger } from "@/components/ai-elements/task";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";
import {
  AlertTriangleIcon,
  BotIcon,
  ChevronsUpDownIcon,
  ChevronRightIcon,
  CircleCheckBigIcon,
  CircleDashedIcon,
  FolderPlusIcon,
  FolderTreeIcon,
  HardDriveIcon,
  Loader2Icon,
  LogOutIcon,
  MessageSquarePlusIcon,
  SettingsIcon,
  SparklesIcon,
} from "lucide-react";

type Viewer = {
  id: string;
  name: string;
  email: string;
  avatarUrl: string | null;
  authMode: "demo" | "workos";
};

type ThreadSummary = {
  id: string;
  title: string;
  updatedAt: string;
  usage: {
    inputTokens: number;
    outputTokens: number;
  };
};

type ThreadPart =
  | {
      type: "text";
      text: string;
    }
  | {
      type: string;
      [key: string]: unknown;
    };

type ThreadMessage = {
  id: string;
  role: "assistant" | "system" | "tool" | "user";
  deletedAt: string | null;
  parts: ThreadPart[];
};

type ThreadDetail = ThreadSummary & {
  messages: ThreadMessage[];
};

type SkillDescriptor = {
  name: string;
  description: string;
};

type Project = {
  name: string;
  fullPath: string;
  source: "mount" | "remote";
  mountId: string | null;
};

type Entity = {
  name: string;
  fullPath: string;
  type: "directory" | "file" | "symlink";
};

type FileTreeResponse = {
  entries: Entity[];
  fullPath: string;
};

type FilePreview = {
  content: string;
  fullPath: string;
};

type HealthPayload = {
  ok: boolean;
  error?: string;
  sandbox?: {
    ok: boolean;
  } | null;
};

type WorkspaceShellProps = {
  apiBaseUrl: string;
  viewer: Viewer;
};

const modelOptions = [
  { inferenceProvider: "openrouter", label: "GPT-4o Mini", modelId: "gpt-4o-mini" },
  { inferenceProvider: "openrouter", label: "GPT-4.1 Mini", modelId: "openai/gpt-4.1-mini" },
  { inferenceProvider: "openrouter", label: "Claude Sonnet 4", modelId: "anthropic/claude-sonnet-4" },
];

async function apiFetch<T>(baseUrl: string, path: string, init?: RequestInit) {
  const response = await fetch(`${baseUrl}${path}`, {
    headers: {
      "content-type": "application/json",
      ...(init?.headers ?? {}),
    },
    ...init,
  });

  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed: ${response.status}`);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

function formatThreadDate(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function extractMessageText(message: ThreadMessage) {
  return message.parts
    .filter((part): part is Extract<ThreadPart, { type: "text" }> => part.type === "text")
    .map((part) => part.text)
    .join("\n\n");
}

function toRelativeProjectPath(projectName: string, fullPath: string) {
  const prefix = `/workspace/projects/${projectName}`;

  if (fullPath === prefix) {
    return ".";
  }

  return fullPath.startsWith(`${prefix}/`) ? fullPath.slice(prefix.length + 1) : fullPath;
}

function getTodoItems(input: {
  currentProject?: string;
  hasMessages: boolean;
  hasThread: boolean;
  health: HealthPayload | null;
  projectCount: number;
}) {
  return [
    {
      id: "sandbox",
      content: "Sandbox heartbeat is live.",
      status: input.health?.ok && input.health?.sandbox?.ok ? "completed" : "pending",
    },
    {
      id: "project",
      content: input.projectCount > 0 ? `Project boundary selected: ${input.currentProject}.` : "Create or mount a project.",
      status: input.projectCount > 0 ? "completed" : "in_progress",
    },
    {
      id: "thread",
      content: input.hasThread ? "Thread memory is active." : "Open a thread before execution.",
      status: input.hasThread ? "completed" : "pending",
    },
    {
      id: "prompt",
      content: input.hasMessages ? "Agent turn executed in the selected project." : "Send the first operator request.",
      status: input.hasMessages ? "completed" : "in_progress",
    },
  ];
}

export function WorkspaceShell({ apiBaseUrl, viewer }: WorkspaceShellProps) {
  const [threads, setThreads] = useState<ThreadSummary[]>([]);
  const [threadDetail, setThreadDetail] = useState<ThreadDetail | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [skills, setSkills] = useState<SkillDescriptor[]>([]);
  const [health, setHealth] = useState<HealthPayload | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedThreadId, setSelectedThreadId] = useState("");
  const [selectedProject, setSelectedProject] = useState("");
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [treeCache, setTreeCache] = useState<Record<string, Entity[]>>({});
  const [selectedPath, setSelectedPath] = useState(".");
  const [selectedFile, setSelectedFile] = useState<FilePreview | null>(null);
  const [creatingProject, setCreatingProject] = useState(false);
  const [sending, setSending] = useState(false);
  const [loadingThread, setLoadingThread] = useState(false);
  const [remoteProjectName, setRemoteProjectName] = useState("workspace");
  const [mountedSourcePath, setMountedSourcePath] = useState("");
  const [mountedProjectName, setMountedProjectName] = useState("local-project");
  const [modelKey, setModelKey] = useState(modelOptions[0]!.modelId);
  const [projectDialogOpen, setProjectDialogOpen] = useState(false);

  const selectedModel = useMemo(
    () => modelOptions.find((option) => option.modelId === modelKey) ?? modelOptions[0]!,
    [modelKey],
  );

  useEffect(() => {
    let cancelled = false;

    async function loadBootData() {
      try {
        const [healthPayload, skillsPayload, projectPayload, threadPayload] = await Promise.all([
          apiFetch<HealthPayload>(apiBaseUrl, "/health"),
          apiFetch<SkillDescriptor[]>(apiBaseUrl, "/skills"),
          apiFetch<Project[]>(apiBaseUrl, "/projects"),
          apiFetch<ThreadSummary[]>(apiBaseUrl, `/threads?userId=${encodeURIComponent(viewer.id)}`),
        ]);

        if (cancelled) {
          return;
        }

        setHealth(healthPayload);
        setSkills(skillsPayload);
        setProjects(projectPayload);
        setThreads(threadPayload);
        setSelectedSkills(skillsPayload.slice(0, 5).map((skill) => skill.name));

        if (projectPayload[0]) {
          setSelectedProject(projectPayload[0].name);
        }

        if (threadPayload[0]) {
          setSelectedThreadId(threadPayload[0].id);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError instanceof Error ? loadError.message : "Failed to load workspace state.");
        }
      }
    }

    void loadBootData();

    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl, viewer.id]);

  useEffect(() => {
    if (!selectedThreadId) {
      setThreadDetail(null);
      return;
    }

    let cancelled = false;

    async function loadThread() {
      setLoadingThread(true);
      try {
        const detail = await apiFetch<ThreadDetail>(
          apiBaseUrl,
          `/threads/${selectedThreadId}?userId=${encodeURIComponent(viewer.id)}`,
        );

        if (!cancelled) {
          setThreadDetail(detail);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError instanceof Error ? loadError.message : "Failed to load thread.");
        }
      } finally {
        if (!cancelled) {
          setLoadingThread(false);
        }
      }
    }

    void loadThread();

    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl, selectedThreadId, viewer.id]);

  useEffect(() => {
    if (!selectedProject) {
      setTreeCache({});
      setSelectedFile(null);
      return;
    }

    let cancelled = false;

    async function loadRootTree() {
      try {
        const listing = await apiFetch<FileTreeResponse>(
          apiBaseUrl,
          `/projects/${selectedProject}/tree?path=${encodeURIComponent(".")}`,
        );

        if (!cancelled) {
          setTreeCache({ ".": listing.entries });
          setSelectedPath(".");
          setSelectedFile(null);
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError instanceof Error ? loadError.message : "Failed to load project tree.");
        }
      }
    }

    void loadRootTree();

    return () => {
      cancelled = true;
    };
  }, [apiBaseUrl, selectedProject]);

  const visibleMessages = useMemo(
    () => threadDetail?.messages.filter((message) => message.deletedAt === null) ?? [],
    [threadDetail],
  );

  const todoItems = useMemo(
    () =>
      getTodoItems({
        currentProject: selectedProject,
        hasMessages: visibleMessages.length > 0,
        hasThread: Boolean(selectedThreadId),
        health,
        projectCount: projects.length,
      }),
    [health, projects.length, selectedProject, selectedThreadId, visibleMessages.length],
  );

  async function refreshProjects(nextProject?: string) {
    const projectPayload = await apiFetch<Project[]>(apiBaseUrl, "/projects");
    setProjects(projectPayload);

    if (nextProject) {
      setSelectedProject(nextProject);
      return;
    }

    if (!selectedProject && projectPayload[0]) {
      setSelectedProject(projectPayload[0].name);
    }
  }

  async function refreshThreads(nextThreadId?: string) {
    const threadPayload = await apiFetch<ThreadSummary[]>(
      apiBaseUrl,
      `/threads?userId=${encodeURIComponent(viewer.id)}`,
    );

    setThreads(threadPayload);

    if (nextThreadId) {
      startTransition(() => {
        setSelectedThreadId(nextThreadId);
      });
    } else if (!selectedThreadId && threadPayload[0]) {
      setSelectedThreadId(threadPayload[0].id);
    }
  }

  async function handleCreateThread() {
    try {
      setError(null);

      const thread = await apiFetch<ThreadSummary>(apiBaseUrl, "/threads", {
        body: JSON.stringify({
          inferenceProvider: selectedModel.inferenceProvider,
          modelId: selectedModel.modelId,
          title: "New thread",
          userId: viewer.id,
        }),
        method: "POST",
      });

      await refreshThreads(thread.id);
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : "Failed to create thread.");
    }
  }

  async function handleCreateRemoteProject() {
    try {
      setCreatingProject(true);
      setError(null);

      const project = await apiFetch<Project>(apiBaseUrl, "/projects/remote", {
        body: JSON.stringify({ projectName: remoteProjectName }),
        method: "POST",
      });

      await refreshProjects(project.name);
      setRemoteProjectName(`${project.name}-next`);
      setProjectDialogOpen(false);
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : "Failed to create project.");
    } finally {
      setCreatingProject(false);
    }
  }

  async function handleMountProject() {
    try {
      setCreatingProject(true);
      setError(null);

      await apiFetch(apiBaseUrl, "/projects/mount", {
        body: JSON.stringify({
          projectName: mountedProjectName,
          sourcePath: mountedSourcePath,
        }),
        method: "POST",
      });

      await refreshProjects(mountedProjectName);
      setProjectDialogOpen(false);
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : "Failed to mount local project.");
    } finally {
      setCreatingProject(false);
    }
  }

  async function handleSelectPath(entity: Entity) {
    if (!selectedProject) {
      return;
    }

    const relativePath = toRelativeProjectPath(selectedProject, entity.fullPath);
    setSelectedPath(relativePath);

    if (entity.type === "directory") {
      if (treeCache[relativePath]) {
        return;
      }

      const listing = await apiFetch<FileTreeResponse>(
        apiBaseUrl,
        `/projects/${selectedProject}/tree?path=${encodeURIComponent(relativePath)}`,
      );

      setTreeCache((current) => ({
        ...current,
        [relativePath]: listing.entries,
      }));

      return;
    }

    const filePayload = await apiFetch<FilePreview>(
      apiBaseUrl,
      `/projects/${selectedProject}/file?path=${encodeURIComponent(relativePath)}`,
    );

    setSelectedFile(filePayload);
  }

  async function handleSend(message: { text: string }) {
    if (!selectedProject) {
      setError("Choose a project before sending a task.");
      return;
    }

    try {
      setSending(true);
      setError(null);

      const response = await apiFetch<{ thread: ThreadDetail }>(apiBaseUrl, "/chat", {
        body: JSON.stringify({
          inferenceProvider: selectedModel.inferenceProvider,
          message: message.text,
          modelId: selectedModel.modelId,
          projectName: selectedProject,
          selectedSkills,
          threadId: selectedThreadId || undefined,
          userId: viewer.id,
        }),
        method: "POST",
      });

      setThreadDetail(response.thread);
      setSelectedThreadId(response.thread.id);
      await refreshThreads(response.thread.id);
    } catch (sendError) {
      setError(sendError instanceof Error ? sendError.message : "Failed to execute agent turn.");
    } finally {
      setSending(false);
    }
  }

  function toggleSkill(skillName: string) {
    setSelectedSkills((current) =>
      current.includes(skillName) ? current.filter((entry) => entry !== skillName) : [...current, skillName],
    );
  }

  function renderTree(path = "."): JSX.Element[] {
    return (treeCache[path] ?? []).map((entity) => {
      const relativePath = toRelativeProjectPath(selectedProject, entity.fullPath);

      if (entity.type === "directory") {
        return (
          <FileTreeFolder key={entity.fullPath} name={entity.name} path={relativePath}>
            {treeCache[relativePath] ? renderTree(relativePath) : null}
          </FileTreeFolder>
        );
      }

      return <FileTreeFile key={entity.fullPath} name={entity.name} path={relativePath} />;
    });
  }

  return (
    <main className="flex h-screen overflow-hidden">
      {/* ── Left Sidebar: threads + navigation ── */}
      <aside className="flex w-[260px] shrink-0 flex-col border-r border-border">
        <div className="border-b border-border px-3 py-3">
          <div className="px-1 pb-3">
            <h1 className="text-lg font-semibold tracking-tight">Protean</h1>
          </div>
          <Button className="w-full" variant="outline" onClick={() => void handleCreateThread()}>
            <MessageSquarePlusIcon className="size-4" />
            New Thread
          </Button>
        </div>

        {/* Thread list */}
        <ScrollArea className="flex-1 px-3">
          <div className="space-y-1 pb-3">
            {threads.map((thread) => (
              <button
                type="button"
                key={thread.id}
                onClick={() => setSelectedThreadId(thread.id)}
                className={cn(
                  "w-full rounded-lg border px-3 py-2.5 text-left transition-colors",
                  selectedThreadId === thread.id
                    ? "border-primary/40 bg-primary/10"
                    : "border-transparent hover:bg-muted",
                )}
              >
                <p className="line-clamp-2 text-sm font-medium">{thread.title}</p>
                <div className="mt-1.5 flex items-center justify-between text-[11px] text-muted-foreground">
                  <span>{formatThreadDate(thread.updatedAt)}</span>
                  <span>{thread.usage.inputTokens + thread.usage.outputTokens} tok</span>
                </div>
              </button>
            ))}
          </div>
        </ScrollArea>

        {/* Bottom pinned: profile menu */}
        <div className="border-t border-border px-3 py-3">
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  className="h-auto w-full justify-start gap-3 px-2 py-2"
                />
              }
            >
              <Avatar className="size-8">
                {viewer.avatarUrl ? <AvatarImage src={viewer.avatarUrl} alt={viewer.name} /> : null}
                <AvatarFallback className="text-xs">{viewer.name.slice(0, 2).toUpperCase()}</AvatarFallback>
              </Avatar>
              <span className="min-w-0 flex-1 truncate text-left text-sm font-medium">{viewer.name}</span>
              <ChevronsUpDownIcon className="size-4 text-muted-foreground" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-56">
              <DropdownMenuItem>
                <SettingsIcon className="size-4" />
                Settings
              </DropdownMenuItem>
              <DropdownMenuItem render={<a href="/logout" />}>
                <LogOutIcon className="size-4" />
                Logout
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </aside>

      {/* ── Center Panel: conversation ── */}
      <section className="flex min-w-0 flex-1 flex-col">
        {/* Error banner */}
        {error ? (
          <div className="border-b border-border px-5 py-3">
            <div className="flex items-center gap-2 rounded-lg border border-destructive/25 bg-destructive/8 px-4 py-2.5 text-sm text-destructive">
              <AlertTriangleIcon className="size-4 shrink-0" />
              {error}
            </div>
          </div>
        ) : null}

        {/* Conversation */}
        <Conversation className="min-h-0 flex-1 px-1">
          <ConversationContent className="pb-6 pt-5">
            {visibleMessages.length === 0 && !loadingThread ? (
              <ConversationEmptyState
                title="No thread output yet"
                description="Create a project, open a thread, and send the operator request that should kick off the agent."
                icon={<SparklesIcon className="size-6" />}
              />
            ) : null}

            {visibleMessages.map((message) => {
              const text = extractMessageText(message) || "_Non-text output omitted._";

              return (
                <Message from={message.role === "tool" ? "assistant" : message.role} key={message.id}>
                  <MessageContent>
                    <MessageResponse>{text}</MessageResponse>
                  </MessageContent>
                </Message>
              );
            })}

            {loadingThread || sending ? (
              <Message from="assistant">
                <MessageContent className="rounded-lg border border-border bg-background px-4 py-3">
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2Icon className="size-4 animate-spin" />
                    Protean is working inside the selected project.
                  </div>
                </MessageContent>
              </Message>
            ) : null}
          </ConversationContent>
          <ConversationScrollButton />
        </Conversation>

        {/* Prompt input */}
        <div className="border-t border-border px-4 py-4">
          <PromptInput onSubmit={(message) => void handleSend(message)} className="rounded-xl border border-border bg-background p-2">
            <PromptInputBody>
              <PromptInputTextarea placeholder={selectedProject ? "Tell Protean what to do in this project." : "Select a project before sending a task."} />
            </PromptInputBody>
            <PromptInputFooter>
              <PromptInputTools className="text-xs text-muted-foreground">
                <Select value={selectedModel.modelId} onValueChange={(value) => value && setModelKey(value)}>
                  <SelectTrigger className="h-8 min-w-40 rounded-lg text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {modelOptions.map((option) => (
                      <SelectItem key={option.modelId} value={option.modelId}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {selectedProject ? (
                  <span className="inline-flex items-center gap-1.5 text-muted-foreground">
                    <BotIcon className="size-3.5" />
                    {selectedSkills.length} skills
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 text-muted-foreground">
                    <ChevronRightIcon className="size-3.5" />
                    Create or mount a project first
                  </span>
                )}
              </PromptInputTools>
              <PromptInputSubmit
                className="rounded-lg"
                disabled={sending || !selectedProject}
                status={sending ? "submitted" : "ready"}
              />
            </PromptInputFooter>
          </PromptInput>
        </div>
      </section>

      {/* ── Right Sidebar: files + skills + todos (stacked) ── */}
      <aside className="flex w-[320px] shrink-0 flex-col border-l border-border">
        <ScrollArea className="flex-1">
          {/* File tree section */}
          <div className="border-b border-border p-4">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-sm font-semibold">File tree</h3>
              <FolderTreeIcon className="size-4 text-muted-foreground" />
            </div>

            <div className="mb-3 flex items-center gap-2">
              <Select value={selectedProject} onValueChange={(value) => value && setSelectedProject(value)}>
                <SelectTrigger className="h-8 flex-1 rounded-lg text-xs">
                  <SelectValue placeholder="Select project" />
                </SelectTrigger>
                <SelectContent>
                  {projects.map((project) => (
                    <SelectItem key={project.fullPath} value={project.name}>
                      {project.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              <Dialog open={projectDialogOpen} onOpenChange={setProjectDialogOpen}>
                <DialogTrigger>
                  <Button size="icon" variant="outline" className="size-8 shrink-0" title="Create project">
                    <FolderPlusIcon className="size-3.5" />
                  </Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Create project</DialogTitle>
                  </DialogHeader>
                  <Tabs defaultValue="remote">
                    <TabsList className="grid w-full grid-cols-2">
                      <TabsTrigger value="remote">Hosted</TabsTrigger>
                      <TabsTrigger value="mount">Self-hosted</TabsTrigger>
                    </TabsList>
                    <TabsContent className="space-y-3 pt-3" value="remote">
                      <Input value={remoteProjectName} onChange={(event) => setRemoteProjectName(event.target.value)} placeholder="workspace-name" />
                      <Button className="w-full" disabled={creatingProject} onClick={() => void handleCreateRemoteProject()}>
                        {creatingProject ? <Loader2Icon className="size-4 animate-spin" /> : <FolderPlusIcon className="size-4" />}
                        Create hosted project
                      </Button>
                    </TabsContent>
                    <TabsContent className="space-y-3 pt-3" value="mount">
                      <Input
                        value={mountedSourcePath}
                        onChange={(event) => setMountedSourcePath(event.target.value)}
                        placeholder="/Users/you/project"
                      />
                      <Input
                        value={mountedProjectName}
                        onChange={(event) => setMountedProjectName(event.target.value)}
                        placeholder="mounted-project"
                      />
                      <Button className="w-full" disabled={creatingProject || !mountedSourcePath} onClick={() => void handleMountProject()}>
                        {creatingProject ? <Loader2Icon className="size-4 animate-spin" /> : <HardDriveIcon className="size-4" />}
                        Mount local project
                      </Button>
                    </TabsContent>
                  </Tabs>
                </DialogContent>
              </Dialog>
            </div>

            <FileTree
              className="max-h-64 overflow-auto"
              onSelect={(path) => {
                const entry = Object.values(treeCache)
                  .flat()
                  .find((entity) => toRelativeProjectPath(selectedProject, entity.fullPath) === path);

                if (entry) {
                  void handleSelectPath(entry);
                }
              }}
              selectedPath={selectedPath}
            >
              {selectedProject ? renderTree() : null}
            </FileTree>

            {selectedFile ? (
              <div className="mt-3">
                <p className="mb-1.5 text-xs font-medium text-muted-foreground">Preview</p>
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded-lg bg-muted p-3 font-mono text-xs leading-5">
                  {selectedFile.content}
                </pre>
              </div>
            ) : null}
          </div>

          {/* Available skills section */}
          <div className="border-b border-border p-4">
            <h3 className="mb-3 text-sm font-semibold">Available skills</h3>
            <div className="space-y-1.5">
              {skills.map((skill) => {
                const active = selectedSkills.includes(skill.name);

                return (
                  <button
                    type="button"
                    key={skill.name}
                    onClick={() => toggleSkill(skill.name)}
                    className={cn(
                      "w-full rounded-lg border px-3 py-2 text-left transition-colors",
                      active ? "border-primary/40 bg-primary/10" : "border-transparent hover:bg-muted",
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <p className="text-sm font-medium">{skill.name}</p>
                      <Badge variant={active ? "secondary" : "outline"} className="text-[10px]">
                        {active ? "on" : "off"}
                      </Badge>
                    </div>
                    <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">{skill.description}</p>
                  </button>
                );
              })}
            </div>
          </div>

          {/* Todos section */}
          <div className="p-4">
            <h3 className="mb-3 text-sm font-semibold">Todos</h3>
            <div className="space-y-2">
              {todoItems.map((todo) => (
                <Task key={todo.id}>
                  <TaskTrigger title={todo.content} />
                  <TaskContent>
                    <TaskItem className="flex items-center gap-2">
                      {todo.status === "completed" ? (
                        <CircleCheckBigIcon className="size-4 text-primary" />
                      ) : (
                        <CircleDashedIcon className="size-4 text-muted-foreground" />
                      )}
                      <span>{todo.content}</span>
                    </TaskItem>
                  </TaskContent>
                </Task>
              ))}
            </div>
          </div>
        </ScrollArea>
      </aside>
    </main>
  );
}
