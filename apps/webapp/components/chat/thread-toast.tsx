"use client";

import { AlertCircleIcon, CheckCircle2Icon } from "lucide-react";
import type { MessageToastState } from "@/components/chat/state/types";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

interface ThreadToastProps {
  toast: MessageToastState | null;
}

export function ThreadToast({ toast }: ThreadToastProps) {
  if (!toast) {
    return null;
  }

  return (
    <div className="pointer-events-none fixed right-4 bottom-4 z-50 w-[min(360px,calc(100vw-2rem))]">
      <Alert className="border shadow-lg" variant={toast.variant}>
        {toast.variant === "destructive" ? (
          <AlertCircleIcon className="size-4" />
        ) : (
          <CheckCircle2Icon className="size-4" />
        )}
        <AlertTitle>{toast.title}</AlertTitle>
        {toast.description ? (
          <AlertDescription>{toast.description}</AlertDescription>
        ) : null}
      </Alert>
    </div>
  );
}
