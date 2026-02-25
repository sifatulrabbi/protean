"use client";

import type { ModelSelection } from "@protean/model-catalog";
import { ModelSelectorDropdown } from "@/components/chat/model-selector-dropdown";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface ThreadRerunDialogProps {
  currentModelSelection: ModelSelection;
  isBusy: boolean;
  onClose: () => void;
  onConfirm: () => void;
  onRerunModelSelectionChange: (selection: ModelSelection) => void;
  open: boolean;
  rerunMessageId: string | null;
  rerunModelSelection: ModelSelection | null;
}

export function ThreadRerunDialog({
  currentModelSelection,
  isBusy,
  onClose,
  onConfirm,
  onRerunModelSelectionChange,
  open,
  rerunMessageId,
  rerunModelSelection,
}: ThreadRerunDialogProps) {
  return (
    <Dialog
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          onClose();
        }
      }}
      open={open}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Rerun with model</DialogTitle>
          <DialogDescription>
            This updates the thread default model and reruns the response.
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-center justify-between gap-2">
          <span className="text-muted-foreground text-xs">Model</span>
          <ModelSelectorDropdown
            disabled={isBusy}
            onChange={onRerunModelSelectionChange}
            value={rerunModelSelection ?? currentModelSelection}
          />
        </div>

        <DialogFooter>
          <Button onClick={onClose} type="button" variant="outline">
            Cancel
          </Button>
          <Button
            disabled={isBusy || !rerunMessageId}
            onClick={onConfirm}
            type="button"
          >
            Rerun
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
