"use client";

import { useState } from "react";
import {
  BrainIcon,
  CheckIcon,
  ChevronDownIcon,
  SparklesIcon,
  WrenchIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  ModelSelector,
  ModelSelectorContent,
  ModelSelectorEmpty,
  ModelSelectorGroup,
  ModelSelectorInput,
  ModelSelectorItem,
  ModelSelectorList,
  ModelSelectorLogo,
  ModelSelectorLogoGroup,
  ModelSelectorName,
  ModelSelectorSeparator,
  ModelSelectorTrigger,
} from "@/components/ai-elements/model-selector";
import type { ModelSelection } from "@protean/model-catalog";
import { getModelReasoningById } from "@protean/model-catalog";
import { useModelCatalog } from "@/components/chat/model-catalog-provider";

const tokenFormatter = new Intl.NumberFormat("en-US", {
  maximumFractionDigits: 1,
  notation: "compact",
});

const priceFormatter = new Intl.NumberFormat("en-US", {
  currency: "USD",
  maximumFractionDigits: 4,
  minimumFractionDigits: 0,
  style: "currency",
});

function formatTokens(value: number) {
  return `${tokenFormatter.format(value)}`;
}

function formatPrice(value: number | null) {
  if (value === null) return "N/A";
  return `${priceFormatter.format(value)}/1M`;
}

interface ModelSelectorDropdownProps {
  disabled?: boolean;
  maxLabelLength?: number;
  onChange: (selection: ModelSelection) => void;
  triggerMode?: "default" | "icon" | "pill";
  value: ModelSelection;
}

function parseModelLabel(name: string) {
  return name.includes(": ") ? name.split(": ")[1] : name;
}

function clampLabel(label: string, maxLength?: number) {
  if (!maxLength || label.length <= maxLength) return label;
  return `${label.slice(0, maxLength - 1)}\u2026`;
}

export function ModelSelectorDropdown({
  disabled,
  maxLabelLength,
  onChange,
  triggerMode = "default",
  value,
}: ModelSelectorDropdownProps) {
  const providers = useModelCatalog();
  const [open, setOpen] = useState(false);

  const selectedProvider = providers.find((p) => p.id === value.providerId);
  const selectedModel = selectedProvider?.models.find(
    (m) => m.id === value.modelId,
  );
  const triggerLabel = clampLabel(
    parseModelLabel(selectedModel?.name ?? "Select model"),
    maxLabelLength,
  );

  function handleSelect(
    providerId: string,
    modelId: string,
    runtimeProvider: string,
  ) {
    const reasoning = getModelReasoningById(providers, providerId, modelId);
    onChange({
      modelId,
      providerId,
      reasoningBudget: (reasoning?.defaultValue ??
        "none") as ModelSelection["reasoningBudget"],
      runtimeProvider,
    });
    setOpen(false);
  }

  return (
    <ModelSelector open={open} onOpenChange={disabled ? undefined : setOpen}>
      <ModelSelectorTrigger asChild>
        <Button
          aria-label="Select model"
          className={
            triggerMode === "icon"
              ? "h-8 w-8 px-0"
              : "h-8 max-w-25 md:max-w-55 justify-between gap-2 text-xs"
          }
          disabled={disabled}
          size="sm"
          type="button"
          variant="outline"
        >
          {triggerMode === "icon" ? (
            <SparklesIcon className="size-4" />
          ) : (
            <>
              {selectedProvider && (
                <ModelSelectorLogo
                  className="size-3"
                  provider={selectedProvider.id}
                />
              )}
              <span className="truncate">{triggerLabel}</span>
              <ChevronDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
            </>
          )}
        </Button>
      </ModelSelectorTrigger>

      <ModelSelectorContent title="Select a model">
        <ModelSelectorInput placeholder="Search models..." />
        <ModelSelectorList>
          <ModelSelectorEmpty>No models found.</ModelSelectorEmpty>
          {providers.map((provider, index) => (
            <div key={provider.id}>
              {index > 0 && <ModelSelectorSeparator />}
              <ModelSelectorGroup heading={provider.name}>
                {provider.models.map((model) => {
                  const isSelected =
                    provider.id === value.providerId &&
                    model.id === value.modelId;
                  return (
                    <ModelSelectorItem
                      key={`${provider.id}-${model.id}`}
                      value={`${provider.name} ${model.name}`}
                      onSelect={() =>
                        handleSelect(
                          provider.id,
                          model.id,
                          model.runtimeProvider,
                        )
                      }
                    >
                      <ModelSelectorLogoGroup>
                        <ModelSelectorLogo provider={provider.id} />
                      </ModelSelectorLogoGroup>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5">
                          <ModelSelectorName>{model.name}</ModelSelectorName>
                          {model.features.tools && (
                            <WrenchIcon
                              aria-label="Supports tool use"
                              className="size-3 shrink-0 text-muted-foreground/60"
                            />
                          )}
                          {model.features.reasoning && (
                            <BrainIcon
                              aria-label="Supports reasoning"
                              className="size-3 shrink-0 text-muted-foreground/60"
                            />
                          )}
                        </div>
                        <p className="truncate text-[11px] text-muted-foreground">
                          {`${formatTokens(model.contextLimits.total)} ctx · In ${formatPrice(model.pricing.inputUsdPerMillion)} · Out ${formatPrice(model.pricing.outputUsdPerMillion)}`}
                        </p>
                      </div>
                      {isSelected && (
                        <CheckIcon className="size-4 shrink-0 text-muted-foreground" />
                      )}
                    </ModelSelectorItem>
                  );
                })}
              </ModelSelectorGroup>
            </div>
          ))}
        </ModelSelectorList>
      </ModelSelectorContent>
    </ModelSelector>
  );
}
