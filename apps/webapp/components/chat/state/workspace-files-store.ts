"use client";

import { create } from "zustand";
import type {
  StorageType,
  WorkspaceFilesState,
  WorkspaceProject,
  WorkspaceProjectTreeNode,
  WorkspaceSkill,
} from "@/components/chat/state/types";

const DEFAULT_PROJECT_NAME = "Default";

interface WorkspaceFilesStore extends WorkspaceFilesState {
  hydrateSelectedProjectName: (projectName?: string) => void;
  reset: () => void;
  setProjectTree: (tree: WorkspaceProjectTreeNode[]) => void;
  setProjectTreeError: (error: string | null) => void;
  setProjectTreeLoading: (loading: boolean) => void;
  setProjects: (projects: WorkspaceProject[]) => void;
  setProjectsError: (error: string | null) => void;
  setProjectsLoading: (loading: boolean) => void;
  setSelectedFileWorkspacePath: (workspacePath: string | null) => void;
  setSelectedProjectName: (projectName: string) => void;
  setSkills: (skills: WorkspaceSkill[]) => void;
  setSkillsError: (error: string | null) => void;
  setSkillsLoading: (loading: boolean) => void;
  setStorageType: (storageType: StorageType) => void;
}

const initialState: WorkspaceFilesState = {
  projectTree: [],
  projectTreeError: null,
  projectTreeLoading: false,
  projects: [],
  projectsError: null,
  projectsLoading: false,
  selectedFileWorkspacePath: null,
  selectedProjectName: DEFAULT_PROJECT_NAME,
  skills: [],
  skillsError: null,
  skillsLoading: false,
  storageType: "cloud",
};

function normalizeProjectName(name?: string): string {
  const trimmed = typeof name === "string" ? name.trim() : "";
  return trimmed || DEFAULT_PROJECT_NAME;
}

function resolveSelectedProjectName(
  projects: WorkspaceProject[],
  selectedProjectName: string,
): string {
  if (projects.some((project) => project.name === selectedProjectName)) {
    return selectedProjectName;
  }

  if (projects.some((project) => project.name === DEFAULT_PROJECT_NAME)) {
    return DEFAULT_PROJECT_NAME;
  }

  return projects[0]?.name ?? selectedProjectName;
}

export const useWorkspaceFilesStore = create<WorkspaceFilesStore>()((set) => ({
  ...initialState,

  hydrateSelectedProjectName: (projectName) =>
    set({ selectedProjectName: normalizeProjectName(projectName) }),

  reset: () => set(() => ({ ...initialState })),

  setProjectTree: (projectTree) => set({ projectTree }),

  setProjectTreeError: (projectTreeError) => set({ projectTreeError }),

  setProjectTreeLoading: (projectTreeLoading) => set({ projectTreeLoading }),

  setProjects: (projects) =>
    set((state) => ({
      projects,
      selectedProjectName: resolveSelectedProjectName(
        projects,
        state.selectedProjectName,
      ),
    })),

  setProjectsError: (projectsError) => set({ projectsError }),

  setProjectsLoading: (projectsLoading) => set({ projectsLoading }),

  setSelectedFileWorkspacePath: (selectedFileWorkspacePath) =>
    set({ selectedFileWorkspacePath }),

  setSelectedProjectName: (selectedProjectName) =>
    set({
      selectedProjectName: normalizeProjectName(selectedProjectName),
      selectedFileWorkspacePath: null,
    }),

  setSkills: (skills) => set({ skills }),

  setSkillsError: (skillsError) => set({ skillsError }),

  setSkillsLoading: (skillsLoading) => set({ skillsLoading }),

  setStorageType: (storageType) =>
    set({
      projectTreeError: null,
      storageType,
      projectsError: null,
      skillsError: null,
    }),
}));
