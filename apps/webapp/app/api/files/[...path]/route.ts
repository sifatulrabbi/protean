import { NextRequest, NextResponse } from "next/server";
import { requireUserId } from "@/lib/server/auth-user";
import { createRemoteFs } from "@protean/vfs";

const MIME_MAP: Record<string, string> = {
  txt: "text/plain",
  md: "text/markdown",
  json: "application/json",
  csv: "text/csv",
  html: "text/html",
  yaml: "text/yaml",
  yml: "text/yaml",
  xml: "text/xml",
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
  svg: "image/svg+xml",
  pdf: "application/pdf",
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
  zip: "application/zip",
};

function getMimeType(filePath: string): string {
  const ext = filePath.split(".").pop()?.toLowerCase() ?? "";
  return MIME_MAP[ext] ?? "application/octet-stream";
}

async function createFs(userId: string) {
  return await createRemoteFs({
    baseUrl: process.env.VFS_SERVER_URL!,
    serviceToken: process.env.VFS_SERVICE_TOKEN!,
    userId,
  });
}

export async function GET(
  _request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const userId = await requireUserId();
  if (!userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { path: pathSegments } = await params;
  const filePath = decodeURIComponent(pathSegments.join("/"));
  const fs = await createFs(userId);

  try {
    const fileStat = await fs.stat(filePath);
    if (fileStat.isDirectory) {
      return NextResponse.json(
        { error: "Cannot serve a directory" },
        { status: 400 },
      );
    }

    const buffer = await fs.readFileBuffer(filePath);
    const mimeType = getMimeType(filePath);
    const fileName = filePath.split("/").pop() ?? "file";

    return new NextResponse(new Uint8Array(buffer), {
      headers: {
        "Content-Type": mimeType,
        "Content-Disposition": `inline; filename="${fileName}"`,
        "Content-Length": String(buffer.length),
        "Cache-Control": "private, no-cache",
      },
    });
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Failed to read file";
    if (message.includes("NOT_FOUND") || message.includes("not found")) {
      return NextResponse.json({ error: "File not found" }, { status: 404 });
    }
    if (message.includes("PATH_TRAVERSAL") || message.includes("not allowed")) {
      return NextResponse.json({ error: "Path not allowed" }, { status: 403 });
    }
    return NextResponse.json({ error: "Failed to read file" }, { status: 500 });
  }
}

export async function DELETE(
  _request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const userId = await requireUserId();
  if (!userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { path: pathSegments } = await params;
  const filePath = decodeURIComponent(pathSegments.join("/"));
  const fs = await createFs(userId);

  try {
    await fs.remove(filePath);
    return NextResponse.json({ deleted: true });
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "Failed to delete";
    if (message.includes("NOT_FOUND") || message.includes("not found")) {
      return NextResponse.json({ error: "File not found" }, { status: 404 });
    }
    return NextResponse.json({ error: "Failed to delete" }, { status: 500 });
  }
}

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const userId = await requireUserId();
  if (!userId) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { path: pathSegments } = await params;
  const filePath = decodeURIComponent(pathSegments.join("/"));

  let newName: string;
  try {
    const body = await request.json();
    newName = String(body.newName ?? "").trim();
    if (!newName || newName.includes("/") || newName.includes("\\")) {
      return NextResponse.json({ error: "Invalid name" }, { status: 400 });
    }
  } catch {
    return NextResponse.json(
      { error: "Invalid request body" },
      { status: 400 },
    );
  }

  try {
    const vfsUrl = process.env.VFS_SERVER_URL!;
    const res = await fetch(`${vfsUrl}/api/v1/files/rename`, {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${process.env.VFS_SERVICE_TOKEN!}`,
        "X-User-Id": userId,
      },
      body: JSON.stringify({ path: filePath, newName }),
    });

    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as {
        error?: { message?: string };
      } | null;
      const msg = body?.error?.message ?? "Failed to rename";
      if (res.status === 404) {
        return NextResponse.json({ error: "File not found" }, { status: 404 });
      }
      return NextResponse.json({ error: msg }, { status: res.status });
    }

    return NextResponse.json({ renamed: true, newName });
  } catch {
    return NextResponse.json({ error: "Failed to rename" }, { status: 500 });
  }
}
