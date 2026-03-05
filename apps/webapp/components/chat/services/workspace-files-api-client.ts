import type {
  WorkspaceProject,
  WorkspaceProjectTreeNode,
  WorkspaceSkill,
} from "@/components/chat/state/types";

interface ProjectsResponse {
  projects?: WorkspaceProject[];
}

interface ProjectResponse {
  project?: WorkspaceProject;
}

interface SkillsResponse {
  skills?: WorkspaceSkill[];
}

interface ProjectFilesResponse {
  entries?: WorkspaceProjectTreeNode[];
  truncated?: boolean;
}

export async function listWorkspaceProjects(): Promise<WorkspaceProject[]> {
  const response = await fetch("/api/workspace/projects");
  if (!response.ok) {
    return [];
  }

  const data = (await response.json()) as ProjectsResponse;
  return data.projects ?? [];
}

export async function createWorkspaceProject(
  name: string,
): Promise<WorkspaceProject | null> {
  const response = await fetch("/api/workspace/projects", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ name }),
  });

  if (!response.ok) {
    return null;
  }

  const data = (await response.json()) as ProjectResponse;
  return data.project ?? null;
}

export async function listWorkspaceSkills(): Promise<WorkspaceSkill[]> {
  const response = await fetch("/api/workspace/skills");
  if (!response.ok) {
    return [];
  }

  const data = (await response.json()) as SkillsResponse;
  return data.skills ?? [];
}

export async function listWorkspaceProjectFiles(
  projectName: string,
): Promise<{ entries: WorkspaceProjectTreeNode[]; truncated: boolean }> {
  const response = await fetch(
    `/api/workspace/projects/${encodeURIComponent(projectName)}/files`,
  );
  if (!response.ok) {
    throw new Error("Failed to list project files");
  }

  const data = (await response.json()) as ProjectFilesResponse;
  return {
    entries: data.entries ?? [],
    truncated: data.truncated === true,
  };
}

export async function deleteWorkspaceFile(
  workspacePath: string,
): Promise<void> {
  const response = await fetch(
    `/api/files/${encodeURIComponent(workspacePath)}`,
    { method: "DELETE" },
  );
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(
      (data as { error?: string }).error ?? "Failed to delete file",
    );
  }
}

export async function renameWorkspaceFile(
  workspacePath: string,
  newName: string,
): Promise<void> {
  const response = await fetch(
    `/api/files/${encodeURIComponent(workspacePath)}`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ newName }),
    },
  );
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new Error(
      (data as { error?: string }).error ?? "Failed to rename file",
    );
  }
}
