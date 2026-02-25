"use client";

import { BrainIcon, CheckIcon } from "lucide-react";
import type { ModelSelection } from "@protean/model-catalog";
import { ModelSelectorDropdown } from "@/components/chat/model-selector-dropdown";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

interface ThreadEditPanelProps {
  activeEditReasoningBudget: string;
  availableEditReasoningBudgets: string[];
  editValue: string;
  hasMessageId: boolean;
  isBusy: boolean;
  onCancel: () => void;
  onModelSelectionChange: (selection: ModelSelection) => void;
  onReasoningBudgetChange: (value: string) => void;
  onSave: () => void;
  onValueChange: (value: string) => void;
  supportsEditThinking: boolean;
  value: ModelSelection;
}

export function ThreadEditPanel({
  activeEditReasoningBudget,
  availableEditReasoningBudgets,
  editValue,
  hasMessageId,
  isBusy,
  onCancel,
  onModelSelectionChange,
  onReasoningBudgetChange,
  onSave,
  onValueChange,
  supportsEditThinking,
  value,
}: ThreadEditPanelProps) {
  return (
    <div className="space-y-2">
      <Textarea
        autoFocus
        className="font-(family-name:--font-literata) text-base"
        onChange={(event) => onValueChange(event.target.value)}
        value={editValue}
      />

      <div className="flex items-center justify-between gap-2">
        <span className="text-muted-foreground text-xs">Model for rerun</span>
        <div className="flex items-center gap-2">
          <ModelSelectorDropdown
            disabled={isBusy}
            onChange={onModelSelectionChange}
            value={value}
          />
          {supportsEditThinking ? (
            <>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    className="h-8 w-8 px-0 sm:hidden"
                    disabled={isBusy}
                    size="sm"
                    type="button"
                    variant="outline"
                  >
                    <BrainIcon className="size-3.5" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {availableEditReasoningBudgets.map((budget) => (
                    <DropdownMenuItem
                      className="flex items-center justify-between gap-2"
                      key={budget}
                      onSelect={() => onReasoningBudgetChange(budget)}
                    >
                      <span>
                        {budget === "none"
                          ? "None"
                          : `${budget[0]?.toUpperCase()}${budget.slice(1)}`}
                      </span>
                      {budget === activeEditReasoningBudget ? (
                        <CheckIcon className="size-4 text-muted-foreground" />
                      ) : null}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>

              <Select
                onValueChange={(nextValue) =>
                  onReasoningBudgetChange(nextValue)
                }
                value={activeEditReasoningBudget}
              >
                <SelectTrigger
                  className="hidden h-8 w-30 gap-2 text-xs sm:flex"
                  size="sm"
                >
                  <BrainIcon className="size-3.5" />
                  <SelectValue placeholder="Thinking" />
                </SelectTrigger>
                <SelectContent align="start">
                  {availableEditReasoningBudgets.map((budget) => (
                    <SelectItem key={budget} value={budget}>
                      {budget === "none"
                        ? "None"
                        : `${budget[0]?.toUpperCase()}${budget.slice(1)}`}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </>
          ) : null}
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Button
          disabled={isBusy || !editValue.trim() || !hasMessageId}
          onClick={onSave}
          size="sm"
          type="button"
        >
          Save
        </Button>
        <Button onClick={onCancel} size="sm" type="button" variant="outline">
          Cancel
        </Button>
      </div>
    </div>
  );
}
