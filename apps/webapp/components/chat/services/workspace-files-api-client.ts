import type { FileEntry } from "@/components/chat/file-entry-context-menu";

interface ListEntriesResponse {
  entries?: FileEntry[];
}

export function downloadUrl(path: string): string {
  return `/api/files/${encodeURIComponent(path)}`;
}

export async function listEntries(dir: string): Promise<FileEntry[]> {
  const response = await fetch(`/api/files?dir=${encodeURIComponent(dir)}`);
  if (!response.ok) {
    return [];
  }

  const data = (await response.json()) as ListEntriesResponse;
  return data.entries ?? [];
}

export async function deleteEntry(path: string): Promise<boolean> {
  const response = await fetch(downloadUrl(path), {
    method: "DELETE",
  });

  return response.ok;
}

export async function renameEntry(
  path: string,
  newName: string,
): Promise<boolean> {
  const response = await fetch(downloadUrl(path), {
    body: JSON.stringify({ newName }),
    headers: { "Content-Type": "application/json" },
    method: "PATCH",
  });

  return response.ok;
}
