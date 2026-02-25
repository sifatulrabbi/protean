"use client";

import { useMemo, type ReactNode } from "react";
import {
  CheckIcon,
  PaperclipIcon,
  BrainIcon,
  MoreHorizontalIcon,
} from "lucide-react";
import { getModelById } from "@protean/model-catalog";
import { useModelCatalog } from "@/components/chat/model-catalog-provider";
import { ModelSelectorDropdown } from "@/components/chat/model-selector-dropdown";
import { useThreadActions } from "@/components/chat/actions/use-thread-actions";
import { useComposerStore } from "@/components/chat/state/composer-store";
import {
  selectIsBusy,
  useThreadSessionStore,
} from "@/components/chat/state/thread-session-store";
import { useRenderCountDebug } from "@/components/chat/utils/use-render-count-debug";
import {
  PromptInput,
  PromptInputBody,
  PromptInputFooter,
  PromptInputSubmit,
  PromptInputTextarea,
} from "@/components/ai-elements/prompt-input";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";

function PromptActionRow({
  children,
  label,
}: {
  children: ReactNode;
  label: string;
}) {
  return (
    <div className="flex items-center justify-between gap-2 py-1">
      <span className="text-sm">{label}</span>
      {children}
    </div>
  );
}

export function ThreadPromptInput() {
  useRenderCountDebug("ThreadPromptInput");

  const { changeModel, changeReasoningBudget, stop, submitPrompt } =
    useThreadActions();
  const providers = useModelCatalog();

  const modelSelection = useComposerStore((state) => state.modelSelection);
  const isDeepResearchEnabled = useComposerStore(
    (state) => state.deepResearchEnabled,
  );
  const isImageCreationEnabled = useComposerStore(
    (state) => state.imageCreationEnabled,
  );
  const toggleDeepResearch = useComposerStore(
    (state) => state.toggleDeepResearch,
  );
  const toggleImageCreation = useComposerStore(
    (state) => state.toggleImageCreation,
  );

  const status = useThreadSessionStore((state) => state.status);
  const isBusy = useThreadSessionStore(selectIsBusy);

  const selectedModel = useMemo(
    () =>
      getModelById(
        providers,
        modelSelection.providerId,
        modelSelection.modelId,
      ),
    [modelSelection.modelId, modelSelection.providerId, providers],
  );
  const availableReasoningBudgets = useMemo(
    () => selectedModel?.reasoning.budgets ?? [],
    [selectedModel],
  );
  const supportsThinking = availableReasoningBudgets.some(
    (budget) => budget !== "none",
  );

  return (
    <div className="sticky bottom-0 bg-background pt-2 pb-4">
      <PromptInput
        onSubmit={async (message) => {
          await submitPrompt({ text: message.text });
        }}
      >
        <PromptInputBody>
          <PromptInputTextarea
            disabled={isBusy}
            placeholder="Ask anything..."
          />
        </PromptInputBody>

        <PromptInputFooter>
          <div className="flex items-center gap-2">
            <Popover>
              <PopoverTrigger asChild>
                <Button size="sm" type="button" variant="outline">
                  <MoreHorizontalIcon className="size-4" />
                </Button>
              </PopoverTrigger>
              <PopoverContent align="start" className="w-80 p-3">
                <div className="space-y-2">
                  <PromptActionRow label="Attach files">
                    <Button disabled size="sm" type="button" variant="outline">
                      <PaperclipIcon className="size-4" />
                      Attach
                    </Button>
                  </PromptActionRow>
                  <PromptActionRow label="Deep Research">
                    <Switch
                      checked={isDeepResearchEnabled}
                      onCheckedChange={toggleDeepResearch}
                    />
                  </PromptActionRow>
                  <PromptActionRow label="Image creation">
                    <Switch
                      checked={isImageCreationEnabled}
                      onCheckedChange={toggleImageCreation}
                    />
                  </PromptActionRow>
                </div>
              </PopoverContent>
            </Popover>

            <ModelSelectorDropdown
              disabled={isBusy}
              maxLabelLength={28}
              onChange={changeModel}
              triggerMode="pill"
              value={modelSelection}
            />

            {supportsThinking ? (
              <>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      className="h-8 w-8 px-0 sm:hidden"
                      size="sm"
                      type="button"
                      variant="outline"
                    >
                      <BrainIcon className="size-3.5" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    {availableReasoningBudgets.map((budget) => (
                      <DropdownMenuItem
                        className="flex items-center justify-between gap-2"
                        key={budget}
                        onSelect={() => changeReasoningBudget(budget)}
                      >
                        <span>
                          {budget === "none"
                            ? "None"
                            : `${budget[0]?.toUpperCase()}${budget.slice(1)}`}
                        </span>
                        {budget === modelSelection.reasoningBudget ? (
                          <CheckIcon className="size-4 text-muted-foreground" />
                        ) : null}
                      </DropdownMenuItem>
                    ))}
                  </DropdownMenuContent>
                </DropdownMenu>

                <Select
                  onValueChange={(value) => changeReasoningBudget(value)}
                  value={modelSelection.reasoningBudget}
                >
                  <SelectTrigger
                    className="hidden h-8 w-30 gap-2 rounded-md text-xs sm:flex"
                    size="sm"
                  >
                    <BrainIcon className="size-3.5" />
                    <SelectValue placeholder="Thinking" />
                  </SelectTrigger>
                  <SelectContent align="start">
                    {availableReasoningBudgets.map((budget) => (
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

          <PromptInputSubmit disabled={isBusy} onStop={stop} status={status} />
        </PromptInputFooter>
      </PromptInput>
    </div>
  );
}
