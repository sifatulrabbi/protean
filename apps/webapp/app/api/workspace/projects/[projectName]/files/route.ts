import { NextResponse } from "next/server";
import { ensureWorkspaceProject } from "@protean/sandbox-client";

import { requireUserId } from "@/lib/server/auth-user";
import { getWorkspaceSandbox } from "@/lib/server/workspace-fs";

const MAX_TREE_DEPTH = 8;
const MAX_TREE_ENTRIES = 2_000;

interface ProjectTreeNode {
  children?: ProjectTreeNode[];
  isDirectory: boolean;
  modified?: string;
  name: string;
  projectPath: string;
  size?: number;
  workspacePath: string;
}

interface BuildTreeState {
  entryCount: number;
  reachedLimit: boolean;
}

function joinPath(base: string, name: string): string {
  return base ? `${base}/${name}` : name;
}

async function buildTree(
  fs: Awaited<ReturnType<typeof getWorkspaceSandbox>>["fs"],
  currentProjectPath: string,
  currentWorkspacePath: string,
  depth: number,
  state: BuildTreeState,
): Promise<ProjectTreeNode[]> {
  if (depth > MAX_TREE_DEPTH || state.reachedLimit) {
    return [];
  }

  const entries = await fs.readdir(currentWorkspacePath);
  const sorted = [...entries].sort((a, b) => {
    if (a.isDirectory && !b.isDirectory) return -1;
    if (!a.isDirectory && b.isDirectory) return 1;
    return a.name.localeCompare(b.name);
  });

  const nodes: ProjectTreeNode[] = [];

  for (const entry of sorted) {
    if (state.entryCount >= MAX_TREE_ENTRIES) {
      state.reachedLimit = true;
      break;
    }

    const nextProjectPath = joinPath(currentProjectPath, entry.name);
    const nextWorkspacePath = joinPath(currentWorkspacePath, entry.name);

    state.entryCount += 1;

    let size: number | undefined;
    let modified: string | undefined;
    try {
      const stat = await fs.stat(nextWorkspacePath);
      size = stat.size;
      modified = stat.modified;
    } catch {
      size = undefined;
      modified = undefined;
    }

    const node: ProjectTreeNode = {
      isDirectory: entry.isDirectory,
      name: entry.name,
      projectPath: nextProjectPath,
      workspacePath: nextWorkspacePath,
      ...(typeof size === "number" ? { size } : {}),
      ...(typeof modified === "string" ? { modified } : {}),
    };

    if (entry.isDirectory) {
      node.children = await buildTree(
        fs,
        nextProjectPath,
        nextWorkspacePath,
        depth + 1,
        state,
      );
    }

    nodes.push(node);

    if (state.reachedLimit) {
      break;
    }
  }

  return nodes;
}

export async function GET(
  _request: Request,
  { params }: { params: Promise<{ projectName: string }> },
) {
  const userId = await requireUserId();
  if (!userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { projectName: rawProjectName } = await params;
  const projectName = decodeURIComponent(rawProjectName ?? "").trim();
  if (!projectName) {
    return NextResponse.json({ error: "Invalid project name" }, { status: 400 });
  }

  try {
    const { fs, workspaceMountPath } = await getWorkspaceSandbox(userId);
    const project = await ensureWorkspaceProject(
      {
        fs,
        workspaceMountPath,
      },
      { name: projectName },
    );

    const state: BuildTreeState = {
      entryCount: 0,
      reachedLimit: false,
    };

    const entries = await buildTree(fs, "", project.relativePath, 1, state);

    return NextResponse.json(
      {
        entries,
        project: {
          ...project,
        },
        truncated: state.reachedLimit,
      },
      {
        headers: { "Cache-Control": "private, no-cache" },
      },
    );
  } catch (error) {
    const message = error instanceof Error ? error.message : "Failed to list project files";
    return NextResponse.json(
      { error: message },
      { status: message.toLowerCase().includes("invalid project") ? 400 : 500 },
    );
  }
}
