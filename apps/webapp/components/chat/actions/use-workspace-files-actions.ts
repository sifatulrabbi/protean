"use client";

import { useCallback } from "react";
import { updateThreadSettings } from "@/components/chat/services/thread-api-client";
import {
  createWorkspaceProject,
  deleteWorkspaceFile,
  listWorkspaceProjectFiles,
  listWorkspaceProjects,
  listWorkspaceSkills,
  renameWorkspaceFile,
} from "@/components/chat/services/workspace-files-api-client";
import { useThreadSessionStore } from "@/components/chat/state/thread-session-store";
import { useWorkspaceFilesStore } from "@/components/chat/state/workspace-files-store";

async function persistThreadProject(projectName: string): Promise<void> {
  const threadId = useThreadSessionStore.getState().activeThreadId;
  if (!threadId) {
    return;
  }

  await updateThreadSettings({
    threadId,
    projectName,
  });
}

export function useWorkspaceFilesActions() {
  const refreshProjects = useCallback(async (): Promise<void> => {
    const store = useWorkspaceFilesStore.getState();

    store.setProjectsLoading(true);
    store.setProjectsError(null);

    try {
      const projects = await listWorkspaceProjects();
      store.setProjects(projects);
    } catch {
      store.setProjects([]);
      store.setProjectsError("Failed to load projects");
    } finally {
      store.setProjectsLoading(false);
    }
  }, []);

  const refreshSkills = useCallback(async (): Promise<void> => {
    const store = useWorkspaceFilesStore.getState();

    store.setSkillsLoading(true);
    store.setSkillsError(null);

    try {
      const skills = await listWorkspaceSkills();
      store.setSkills(skills);
    } catch {
      store.setSkills([]);
      store.setSkillsError("Failed to load skills");
    } finally {
      store.setSkillsLoading(false);
    }
  }, []);

  const refreshProjectTree = useCallback(
    async (projectName?: string): Promise<void> => {
      const store = useWorkspaceFilesStore.getState();
      const targetProjectName = (projectName ?? store.selectedProjectName).trim();

      if (!targetProjectName) {
        store.setProjectTree([]);
        return;
      }

      store.setProjectTreeLoading(true);
      store.setProjectTreeError(null);

      try {
        const { entries } = await listWorkspaceProjectFiles(targetProjectName);
        store.setProjectTree(entries);
      } catch {
        store.setProjectTree([]);
        store.setProjectTreeError("Failed to load project files");
      } finally {
        store.setProjectTreeLoading(false);
      }
    },
    [],
  );

  const selectProject = useCallback(
    async (projectName: string): Promise<void> => {
      const store = useWorkspaceFilesStore.getState();
      store.setSelectedProjectName(projectName);
      await refreshProjectTree(projectName);

      try {
        await persistThreadProject(projectName);
      } catch {
        store.setProjectsError("Failed to update active thread project");
      }
    },
    [refreshProjectTree],
  );

  const createProject = useCallback(
    async (name: string): Promise<boolean> => {
      const store = useWorkspaceFilesStore.getState();
      store.setProjectsError(null);

      try {
        const project = await createWorkspaceProject(name);
        if (!project) {
          store.setProjectsError("Failed to create project");
          return false;
        }

        await refreshProjects();
        await selectProject(project.name);
        return true;
      } catch {
        store.setProjectsError("Failed to create project");
        return false;
      }
    },
    [refreshProjects, selectProject],
  );

  const deleteFile = useCallback(
    async (workspacePath: string): Promise<boolean> => {
      try {
        await deleteWorkspaceFile(workspacePath);
        await refreshProjectTree();
        return true;
      } catch {
        return false;
      }
    },
    [refreshProjectTree],
  );

  const renameFile = useCallback(
    async (workspacePath: string, newName: string): Promise<boolean> => {
      try {
        await renameWorkspaceFile(workspacePath, newName);
        await refreshProjectTree();
        return true;
      } catch {
        return false;
      }
    },
    [refreshProjectTree],
  );

  return {
    createProject,
    deleteFile,
    refreshProjectTree,
    refreshProjects,
    refreshSkills,
    renameFile,
    selectProject,
  };
}
