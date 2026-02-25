"use client";

import { useThreadSessionStore } from "@/components/chat/state/thread-session-store";

export function ThreadErrorAlert() {
  const error = useThreadSessionStore((state) => state.error);

  if (!error) {
    return null;
  }

  return (
    <div className="mt-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-destructive text-sm">
      {error.message}
    </div>
  );
}
