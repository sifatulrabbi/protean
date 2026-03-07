import { NextResponse } from "next/server";
import { discoverWorkspaceSkills } from "@protean/skill-manifests";

import { requireUserId } from "@/lib/server/auth-user";
import { createWorkspaceFs } from "@/lib/server/workspace-fs";

async function discoverFromWorkspace(userId: string) {
  const fs = await createWorkspaceFs(userId);
  return discoverWorkspaceSkills(fs, {
    skillsDir: "skills",
  });
}

export async function GET() {
  const userId = await requireUserId();

  if (!userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  try {
    const skills = await discoverFromWorkspace(userId);
    return NextResponse.json(
      { skills },
      { headers: { "Cache-Control": "private, no-cache" } },
    );
  } catch {
    return NextResponse.json(
      { error: "Failed to list skills" },
      { status: 500 },
    );
  }
}
