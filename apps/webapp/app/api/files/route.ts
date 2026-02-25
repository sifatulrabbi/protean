import { NextRequest, NextResponse } from "next/server";
import { requireUserId } from "@/lib/server/auth-user";
import { createRemoteFs } from "@protean/vfs";

export async function GET(request: NextRequest) {
  const userId = await requireUserId();
  if (!userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const dir = request.nextUrl.searchParams.get("dir") ?? "/";

  const fs = await createRemoteFs({
    baseUrl: process.env.VFS_SERVER_URL!,
    serviceToken: process.env.VFS_SERVICE_TOKEN!,
    userId,
  });

  try {
    const dirEntries = await fs.readdir(dir);

    const entries = await Promise.all(
      dirEntries.map(async (entry) => {
        const relativePath = dir === "/" ? entry.name : `${dir}/${entry.name}`;
        try {
          const entryStat = await fs.stat(relativePath);
          return {
            name: entry.name,
            path: relativePath,
            isDirectory: entry.isDirectory,
            size: entryStat.size,
            modified: entryStat.modified,
          };
        } catch {
          return {
            name: entry.name,
            path: relativePath,
            isDirectory: entry.isDirectory,
            size: 0,
            modified: new Date().toISOString(),
          };
        }
      }),
    );

    // Sort: directories first, then alphabetical
    entries.sort((a, b) => {
      if (a.isDirectory && !b.isDirectory) return -1;
      if (!a.isDirectory && b.isDirectory) return 1;
      return a.name.localeCompare(b.name);
    });

    return NextResponse.json(
      { entries, dir },
      {
        headers: { "Cache-Control": "private, no-cache" },
      },
    );
  } catch (err: unknown) {
    const message =
      err instanceof Error ? err.message : "Failed to list directory";
    if (message.includes("NOT_FOUND") || message.includes("not found")) {
      return NextResponse.json({ entries: [], dir });
    }
    return NextResponse.json(
      { error: "Failed to list directory" },
      { status: 500 },
    );
  }
}
