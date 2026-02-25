"use client";

import type { ComponentProps, ComponentType, ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";
import { BrainIcon, ChevronDownIcon, DotIcon } from "lucide-react";
import { memo } from "react";

// ChainOfThought is a single Collapsible so that ChainOfThoughtHeader
// (CollapsibleTrigger) and ChainOfThoughtContent (CollapsibleContent) share
// the same Radix context and the same auto-generated aria-controls / content
// id. The previous two-Collapsible design caused aria-controls to point to the
// wrong element and produced React hydration mismatches.

export type ChainOfThoughtProps = ComponentProps<typeof Collapsible>;

export const ChainOfThought = memo(
  ({ className, children, ...props }: ChainOfThoughtProps) => (
    <Collapsible className={cn("not-prose w-full my-2", className)} {...props}>
      {children}
    </Collapsible>
  ),
);

export type ChainOfThoughtHeaderProps = ComponentProps<
  typeof CollapsibleTrigger
>;

export const ChainOfThoughtHeader = memo(
  ({ className, children, ...props }: ChainOfThoughtHeaderProps) => (
    <CollapsibleTrigger
      className={cn(
        "group flex w-auto items-center gap-2 text-muted-foreground text-sm transition-colors hover:text-foreground",
        className,
      )}
      {...props}
    >
      <BrainIcon className="size-4" />
      <span className="flex text-left">{children ?? "Chain of Thought"}</span>
      <ChevronDownIcon className="size-4 transition-transform group-data-[state=open]:rotate-180" />
    </CollapsibleTrigger>
  ),
);

export type ChainOfThoughtStepProps = ComponentProps<"div"> & {
  icon?: ComponentType<{ className?: string }> | null;
  label: ReactNode;
  description?: ReactNode;
  status?: "complete" | "active" | "pending";
};

const stepStatusStyles = {
  active: "text-foreground",
  complete: "text-muted-foreground",
  pending: "text-muted-foreground/50",
};

export const ChainOfThoughtStep = memo(
  ({
    className,
    icon: Icon = DotIcon,
    label,
    description,
    status = "complete",
    children,
    ...props
  }: ChainOfThoughtStepProps) => (
    <div
      className={cn(
        "flex gap-2 text-sm",
        stepStatusStyles[status],
        "fade-in-0 slide-in-from-top-2 animate-in",
        className,
      )}
      {...props}
    >
      {Icon && (
        <div className="relative mt-0.5">
          <Icon className="size-4" />
          <div className="absolute top-7 bottom-0 left-1/2 -mx-px w-px bg-border" />
        </div>
      )}
      <div className="flex-1 space-y-2 overflow-hidden">
        <div>{label}</div>
        {description && (
          <div className="text-muted-foreground text-xs">{description}</div>
        )}
        {children}
      </div>
    </div>
  ),
);

export type ChainOfThoughtSearchResultsProps = ComponentProps<"div">;

export const ChainOfThoughtSearchResults = memo(
  ({ className, ...props }: ChainOfThoughtSearchResultsProps) => (
    <div
      className={cn("flex flex-wrap items-center gap-2", className)}
      {...props}
    />
  ),
);

export type ChainOfThoughtSearchResultProps = ComponentProps<typeof Badge>;

export const ChainOfThoughtSearchResult = memo(
  ({ className, children, ...props }: ChainOfThoughtSearchResultProps) => (
    <Badge
      className={cn("gap-1 px-2 py-0.5 font-normal text-xs", className)}
      variant="secondary"
      {...props}
    >
      {children}
    </Badge>
  ),
);

export type ChainOfThoughtContentProps = ComponentProps<
  typeof CollapsibleContent
>;

export const ChainOfThoughtContent = memo(
  ({ className, children, ...props }: ChainOfThoughtContentProps) => (
    <CollapsibleContent
      className={cn(
        "mt-2 space-y-3",
        "data-[state=closed]:fade-out-0 data-[state=closed]:slide-out-to-top-2 data-[state=open]:slide-in-from-top-2 text-popover-foreground outline-none data-[state=closed]:animate-out data-[state=open]:animate-in",
        className,
      )}
      {...props}
    >
      {children}
    </CollapsibleContent>
  ),
);

export type ChainOfThoughtImageProps = ComponentProps<"div"> & {
  caption?: string;
};

export const ChainOfThoughtImage = memo(
  ({ className, children, caption, ...props }: ChainOfThoughtImageProps) => (
    <div className={cn("mt-2 space-y-2", className)} {...props}>
      <div className="relative flex max-h-88 items-center justify-center overflow-hidden rounded-lg bg-muted p-3">
        {children}
      </div>
      {caption && <p className="text-muted-foreground text-xs">{caption}</p>}
    </div>
  ),
);

ChainOfThought.displayName = "ChainOfThought";
ChainOfThoughtHeader.displayName = "ChainOfThoughtHeader";
ChainOfThoughtStep.displayName = "ChainOfThoughtStep";
ChainOfThoughtSearchResults.displayName = "ChainOfThoughtSearchResults";
ChainOfThoughtSearchResult.displayName = "ChainOfThoughtSearchResult";
ChainOfThoughtContent.displayName = "ChainOfThoughtContent";
ChainOfThoughtImage.displayName = "ChainOfThoughtImage";
