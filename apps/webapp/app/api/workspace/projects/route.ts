import { NextResponse } from "next/server";
import {
  DEFAULT_SANDBOX_PROJECT_NAME,
  ensureWorkspaceProject,
  listWorkspaceProjects,
} from "@protean/sandbox-client";

import { requireUserId } from "@/lib/server/auth-user";
import { getWorkspaceSandbox } from "@/lib/server/workspace-fs";

export async function GET() {
  const userId = await requireUserId();

  if (!userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  try {
    const { fs, workspaceMountPath } = await getWorkspaceSandbox(userId);
    await ensureWorkspaceProject(
      {
        fs,
        workspaceMountPath,
      },
      { name: DEFAULT_SANDBOX_PROJECT_NAME },
    );
    const projects = await listWorkspaceProjects({
      fs,
      workspaceMountPath,
    });

    return NextResponse.json(
      { projects },
      { headers: { "Cache-Control": "private, no-cache" } },
    );
  } catch {
    return NextResponse.json(
      { error: "Failed to list projects" },
      { status: 500 },
    );
  }
}

export async function POST(request: Request) {
  const [userId, body] = await Promise.all([
    requireUserId(),
    request.json().catch(() => null),
  ]);

  if (!userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  if (!body || typeof body.name !== "string") {
    return NextResponse.json(
      { error: "Invalid request body" },
      { status: 400 },
    );
  }

  try {
    const { fs, workspaceMountPath } = await getWorkspaceSandbox(userId);
    const project = await ensureWorkspaceProject(
      {
        fs,
        workspaceMountPath,
      },
      { name: body.name },
    );

    return NextResponse.json({ project }, { status: 201 });
  } catch (error) {
    const message = error instanceof Error ? error.message : "Invalid project";
    return NextResponse.json({ error: message }, { status: 400 });
  }
}
