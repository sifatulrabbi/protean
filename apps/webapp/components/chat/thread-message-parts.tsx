"use client";

import type { UIMessage } from "ai";
import { useMemo } from "react";
import { MessageResponse } from "@/components/ai-elements/message";
import {
  type ThreadToolPart,
  isThreadToolPart,
} from "@/components/chat/thread-ui-shared";
import { FileArtifact } from "@/components/chat/file-artifact";
import { detectFilesFromToolResult } from "@/lib/file-utils";
import { Shimmer } from "@/components/ai-elements/shimmer";
import {
  ChainOfThought,
  ChainOfThoughtContent,
  ChainOfThoughtHeader,
  ChainOfThoughtStep,
} from "@/components/ai-elements/chain-of-thought";
import { WrenchIcon } from "lucide-react";

type ReasoningPart = Extract<UIMessage["parts"][number], { type: "reasoning" }>;
type TextPart = Extract<UIMessage["parts"][number], { type: "text" }>;
type ChainPart = ReasoningPart | ThreadToolPart;

// ─── Part grouping (reasoning/tools vs text) ──────────────────────────────────

type ChainGroup = { type: "chain"; parts: ChainPart[]; startIndex: number };
type TextGroup = { type: "text"; part: TextPart; index: number };
type PartGroup = ChainGroup | TextGroup;

function groupParts(parts: UIMessage["parts"]): PartGroup[] {
  const groups: PartGroup[] = [];
  let currentChain: ChainPart[] | null = null;
  let chainStartIndex = 0;

  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    if (part.type === "reasoning" || isThreadToolPart(part)) {
      if (!currentChain) {
        currentChain = [];
        chainStartIndex = i;
      }
      currentChain.push(part as ChainPart);
    } else if (part.type === "text") {
      if (currentChain) {
        groups.push({
          type: "chain",
          parts: currentChain,
          startIndex: chainStartIndex,
        });
        currentChain = null;
      }
      groups.push({ type: "text", part: part as TextPart, index: i });
    }
  }

  if (currentChain) {
    groups.push({
      type: "chain",
      parts: currentChain,
      startIndex: chainStartIndex,
    });
  }

  return groups;
}

function hasOutputAfterGroup(groups: PartGroup[], index: number): boolean {
  return groups
    .slice(index + 1)
    .some(
      (group) => group.type === "text" && group.part.text.trim().length > 0,
    );
}

// ─── Step collapsing (merge consecutive same-name tool calls into one step) ───

type SingleStep = {
  kind: "single";
  part: ChainPart;
  key: string;
  isLastInChain: boolean;
};

type GroupedToolStep = {
  kind: "grouped";
  toolName: string;
  parts: ThreadToolPart[];
  key: string;
  isLastInChain: boolean;
};

type Step = SingleStep | GroupedToolStep;

function collapseSteps(
  chainParts: ChainPart[],
  messageKey: string,
  startIndex: number,
): Step[] {
  const steps: Step[] = [];
  let i = 0;

  while (i < chainParts.length) {
    const part = chainParts[i];
    const absIdx = startIndex + i;

    if (part.type !== "reasoning" && isThreadToolPart(part as ThreadToolPart)) {
      const toolPart = part as ThreadToolPart;
      const toolName = getToolName(toolPart);

      // Keep file-related tools grouped (existing UI), and render non-file
      // tools ungrouped so each call can show a concise input summary.
      const grouped: ThreadToolPart[] = [toolPart];
      const shouldGroup = isFileToolName(toolName);
      let j = i + 1;
      while (shouldGroup && j < chainParts.length) {
        const next = chainParts[j];
        if (
          next.type !== "reasoning" &&
          isThreadToolPart(next as ThreadToolPart)
        ) {
          const nextName = getToolName(next as ThreadToolPart);
          if (nextName === toolName) {
            grouped.push(next as ThreadToolPart);
            j++;
            continue;
          }
        }
        break;
      }

      const key = toolPart.toolCallId?.trim() || `${messageKey}-tool-${absIdx}`;

      if (grouped.length > 1) {
        steps.push({
          kind: "grouped",
          toolName,
          parts: grouped,
          key,
          isLastInChain: j === chainParts.length,
        });
      } else {
        steps.push({
          kind: "single",
          part: toolPart,
          key,
          isLastInChain: j === chainParts.length,
        });
      }
      i = j;
    } else {
      // reasoning part
      const key = `${messageKey}-reasoning-${absIdx}`;
      steps.push({
        kind: "single",
        part,
        key,
        isLastInChain: i === chainParts.length - 1,
      });
      i++;
    }
  }

  return steps;
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function getToolName(part: ThreadToolPart): string {
  return part.type === "dynamic-tool"
    ? part.toolName
    : part.type.split("-").slice(1).join("-");
}

function isSubAgentToolName(toolName: string): boolean {
  const normalized = toolName.toLowerCase().replace(/[^a-z0-9]/g, "");
  return normalized === "spawnsubagent";
}

function isWebToolName(toolName: string): boolean {
  const normalized = toolName.toLowerCase().replace(/[^a-z0-9]/g, "");
  return (
    normalized === "websearchgeneral" ||
    normalized === "websearchnews" ||
    normalized === "webfetchurlcontent"
  );
}

function isSkillToolName(toolName: string): boolean {
  const normalized = toolName.toLowerCase().replace(/[^a-z0-9]/g, "");
  return normalized === "skill";
}

function isFileToolName(toolName: string): boolean {
  const normalized = toolName.toLowerCase().replace(/[^a-z0-9]/g, "");
  const workspaceToolNames = new Set([
    // New workspace tool names.
    "stat",
    "listdir",
    "readfile",
    "mkdir",
    "writefile",
    "move",
    "remove",
    // Legacy names kept for older transcripts.
    "getfilestat",
    "readdir",
    "getfilecontent",
    "createdirectory",
  ]);
  return workspaceToolNames.has(normalized);
}

function toInputRecord(
  toolPart: ThreadToolPart,
): Record<string, unknown> | null {
  const input = toolPart.input as Record<string, unknown> | null | undefined;
  return input ?? null;
}

function stringOrNull(value: unknown): string | null {
  return typeof value === "string" && value.trim().length > 0
    ? value.trim()
    : null;
}

function toSingleLine(value: string, maxLen = 140): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= maxLen) return normalized;
  return `${normalized.slice(0, maxLen - 1)}…`;
}

function toCompactJson(value: unknown): string | null {
  try {
    return JSON.stringify(value);
  } catch {
    return null;
  }
}

function getChainHeader(parts: ChainPart[], isStreaming: boolean): string {
  if (isStreaming) return "Working...";

  const reasoningCount = parts.filter((p) => p.type === "reasoning").length;
  const toolCount = parts.filter((p) => p.type !== "reasoning").length;

  const thoughtLabel = reasoningCount > 0 ? "Thought" : null;
  const toolLabel =
    toolCount > 0
      ? `${toolCount} tool${toolCount !== 1 ? "s" : ""} used`
      : null;

  return [thoughtLabel, toolLabel].filter(Boolean).join(" • ");
}

function getToolStepStatus(
  part: ThreadToolPart,
  isStreaming: boolean,
): "active" | "complete" | "pending" {
  if (isStreaming) return "active";
  switch (part.state) {
    case "input-streaming":
    case "input-available":
    case "approval-requested":
    case "approval-responded":
      return "active";
    default:
      return "complete";
  }
}

function getGoalDescription(toolPart: ThreadToolPart): string | null {
  const toolName = getToolName(toolPart);
  if (isFileToolName(toolName)) return null;

  const input = toInputRecord(toolPart);
  if (!input) {
    if (
      typeof toolPart.input === "string" &&
      toolPart.input.trim().length > 0
    ) {
      return `Input: ${toSingleLine(toolPart.input)}`;
    }
    return null;
  }

  if (isSubAgentToolName(toolName)) {
    const goal = stringOrNull(input.goal);
    if (goal) return `Goal: ${toSingleLine(goal)}`;
    const fallback = toCompactJson(input);
    return fallback ? `Input: ${toSingleLine(fallback)}` : null;
  }

  if (isSkillToolName(toolName)) {
    const skillId = stringOrNull(input.id);
    if (skillId) return toSingleLine(skillId);
  }

  const normalized = toolName.toLowerCase().replace(/[^a-z0-9]/g, "");
  if (normalized === "websearchgeneral" || normalized === "websearchnews") {
    const query =
      stringOrNull(input.query) ??
      stringOrNull(input.q) ??
      stringOrNull(input.searchQuery);
    if (!query) return null;
    return normalized === "websearchnews"
      ? `Search news for: ${toSingleLine(query)}`
      : `Search web for: ${toSingleLine(query)}`;
  }

  if (normalized === "webfetchurlcontent") {
    const directUrl = stringOrNull(input.url);
    if (directUrl) {
      return `Fetch URL content: ${toSingleLine(directUrl)}`;
    }

    if (Array.isArray(input.urls)) {
      const urls = input.urls
        .filter((value): value is string => typeof value === "string")
        .map((value) => value.trim())
        .filter((value) => value.length > 0);
      if (urls.length === 1) {
        return `Fetch URL content: ${toSingleLine(urls[0] ?? "")}`;
      }
      if (urls.length > 1) {
        return `Fetch URL content from ${urls.length} URLs`;
      }
    }
  }

  const fallback = toCompactJson(input);
  return fallback ? `Input: ${toSingleLine(fallback)}` : null;
}

function getGroupedStatus(
  parts: ThreadToolPart[],
  isStreaming: boolean,
): "active" | "complete" | "pending" {
  if (isStreaming) return "active";
  const anyActive = parts.some(
    (p) =>
      p.state === "input-streaming" ||
      p.state === "input-available" ||
      p.state === "approval-requested" ||
      p.state === "approval-responded",
  );
  return anyActive ? "active" : "complete";
}

function sanitizeIdSegment(value: string): string {
  const sanitized = value.toLowerCase().replace(/[^a-z0-9_-]+/g, "-");
  return sanitized.length > 0 ? sanitized : "item";
}

function ReasoningBulletIcon({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={`inline-flex items-center justify-center leading-none ${className ?? ""}`}
    >
      •
    </span>
  );
}

// ─── Component ────────────────────────────────────────────────────────────────

interface ThreadMessagePartsProps {
  isLastMessage: boolean;
  isStreaming: boolean;
  message: UIMessage;
  messageKey: string;
}

export function ThreadMessageParts({
  isLastMessage,
  isStreaming,
  message,
  messageKey,
}: ThreadMessagePartsProps) {
  const groups = useMemo(() => groupParts(message.parts), [message.parts]);

  return (
    <>
      {groups.map((group, groupIndex) => {
        if (group.type === "text") {
          return (
            <MessageResponse
              key={`${messageKey}-text-${group.index}`}
              className="w-full text-base"
            >
              {group.part.text}
            </MessageResponse>
          );
        }

        const chainParts = group.parts;
        const isLastGroup = groupIndex === groups.length - 1;
        const isChainStreaming = isLastMessage && isStreaming && isLastGroup;
        const hasOutputAfter = hasOutputAfterGroup(groups, groupIndex);
        const steps = collapseSteps(chainParts, messageKey, group.startIndex);
        const chainContentId = `cot-content-${sanitizeIdSegment(messageKey)}-${group.startIndex}-${groupIndex}`;

        return (
          <ChainOfThought
            key={`${messageKey}-chain-${groupIndex}-${hasOutputAfter ? "collapsed" : "live"}`}
            defaultOpen={isLastGroup && isLastMessage}
          >
            <ChainOfThoughtHeader aria-controls={chainContentId}>
              {isChainStreaming ? (
                <Shimmer duration={1.2}>
                  {getChainHeader(chainParts, isChainStreaming)}
                </Shimmer>
              ) : (
                getChainHeader(chainParts, isChainStreaming)
              )}
            </ChainOfThoughtHeader>
            <ChainOfThoughtContent id={chainContentId}>
              {steps.map((step) => {
                const isThisStepStreaming =
                  isChainStreaming && step.isLastInChain;

                // ── Reasoning ──────────────────────────────────────────────
                if (step.kind === "single" && step.part.type === "reasoning") {
                  const reasoning = step.part as ReasoningPart;
                  if (!reasoning.text.trim()) return null;
                  return (
                    <ChainOfThoughtStep
                      key={step.key}
                      icon={ReasoningBulletIcon}
                      label={
                        <div className="whitespace-pre-wrap wrap-break-words">
                          {reasoning.text}
                        </div>
                      }
                      status={isThisStepStreaming ? "active" : "complete"}
                    />
                  );
                }

                // ── Single tool call ───────────────────────────────────────
                if (step.kind === "single") {
                  const toolPart = step.part as ThreadToolPart;
                  const status = getToolStepStatus(
                    toolPart,
                    isThisStepStreaming,
                  );
                  const toolName = getToolName(toolPart);
                  const goalDescription = getGoalDescription(toolPart);
                  const files =
                    toolPart.state === "output-available"
                      ? detectFilesFromToolResult(toolPart.output)
                      : null;

                  return (
                    <ChainOfThoughtStep
                      key={step.key}
                      icon={WrenchIcon}
                      label={
                        isThisStepStreaming ? (
                          <Shimmer duration={1.2}>{toolName}</Shimmer>
                        ) : (
                          toolName
                        )
                      }
                      description={
                        goalDescription?.trim() ? (
                          isThisStepStreaming ? (
                            <Shimmer duration={1.2}>{goalDescription}</Shimmer>
                          ) : (
                            goalDescription
                          )
                        ) : undefined
                      }
                      status={status}
                    >
                      {files && files.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {files.map((file) => (
                            <FileArtifact
                              key={`${step.key}-file-${file.path}`}
                              file={file}
                            />
                          ))}
                        </div>
                      ) : null}
                    </ChainOfThoughtStep>
                  );
                }

                // ── Grouped tool calls (same name, consecutive) ────────────
                const status = getGroupedStatus(
                  step.parts,
                  isThisStepStreaming,
                );
                const allFiles = step.parts.flatMap((p) =>
                  p.state === "output-available"
                    ? (detectFilesFromToolResult(p.output) ?? [])
                    : [],
                );

                return (
                  <ChainOfThoughtStep
                    key={step.key}
                    icon={WrenchIcon}
                    label={
                      isThisStepStreaming ? (
                        <Shimmer duration={1.2}>{step.toolName}</Shimmer>
                      ) : (
                        step.toolName
                      )
                    }
                    status={status}
                  >
                    {allFiles.length > 0 && (
                      <div className="flex flex-wrap gap-1">
                        {allFiles.map((file) => (
                          <FileArtifact
                            key={`${step.key}-file-${file.path}`}
                            file={file}
                          />
                        ))}
                      </div>
                    )}
                  </ChainOfThoughtStep>
                );
              })}
            </ChainOfThoughtContent>
          </ChainOfThought>
        );
      })}
    </>
  );
}
