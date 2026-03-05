import type { UIMessage } from "ai";
import type { UseChatHelpers } from "@ai-sdk/react";
import type { ModelSelection } from "@protean/model-catalog";
import type { ThreadUsage, ThreadRecordTrimmed } from "@protean/agent-memory";
import type { ThreadStatus } from "@/components/chat/thread-ui-shared";

export interface ThreadActionResult {
  error?: Error;
  ok: boolean;
}

export interface ThreadRuntimeBridge {
  regenerate: UseChatHelpers<UIMessage>["regenerate"];
  sendMessage: UseChatHelpers<UIMessage>["sendMessage"];
  setMessages: UseChatHelpers<UIMessage>["setMessages"];
  stop: UseChatHelpers<UIMessage>["stop"];
}

export interface ThreadSessionState {
  activeThreadId: string | null;
  error?: Error;
  isCreatingThread: boolean;
  isPersistingMutation: boolean;
  messageUsageMap: Record<string, ThreadUsage>;
  messages: UIMessage[];
  pendingInvokeHandled: Record<string, true>;
  runtime: ThreadRuntimeBridge | null;
  status: ThreadStatus;
}

export interface ComposerState {
  deepResearchEnabled: boolean;
  imageCreationEnabled: boolean;
  modelSelection: ModelSelection;
}

export interface MessageToastState {
  description?: string;
  title: string;
  variant: "default" | "destructive";
}

export interface MessageUiState {
  copiedMessageId: string | null;
  editingMessageId: string | null;
  editModelSelection: ModelSelection | null;
  editReasoningBudget: string | null;
  editValue: string;
  rerunMessageId: string | null;
  rerunModelSelection: ModelSelection | null;
  toast: MessageToastState | null;
}

export interface ThreadsSidebarState {
  deletingThreadId: string | null;
  mobileOpen: boolean;
  mounted: boolean;
  threadItems: ThreadRecordTrimmed[];
}

export type StorageType = "cloud" | "local";

export interface WorkspaceProject {
  name: string;
  relativePath: string;
  workspaceMountPath: string;
}

export interface WorkspaceProjectTreeNode {
  children?: WorkspaceProjectTreeNode[];
  isDirectory: boolean;
  modified?: string;
  name: string;
  projectPath: string;
  size?: number;
  workspacePath: string;
}

export interface WorkspaceSkill {
  id: string;
  name: string;
  description: string;
  path: string;
  frontmatter: Record<string, unknown>;
  metadata?: Record<string, string>;
}

export interface WorkspaceFilesState {
  projectTree: WorkspaceProjectTreeNode[];
  projectTreeError: string | null;
  projectTreeLoading: boolean;
  projects: WorkspaceProject[];
  projectsError: string | null;
  projectsLoading: boolean;
  selectedFileWorkspacePath: string | null;
  selectedProjectName: string;
  skills: WorkspaceSkill[];
  skillsError: string | null;
  skillsLoading: boolean;
  storageType: StorageType;
}
