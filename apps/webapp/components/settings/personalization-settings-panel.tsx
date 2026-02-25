"use client";

import { useMemo, useState } from "react";
import { BrainIcon, Edit3Icon, SaveIcon, XIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";

type MemoryItem = {
  id: string;
  summary: string;
  updatedAt: string;
};

const DUMMY_MEMORIES: MemoryItem[] = [
  {
    id: "mem-1",
    summary: "User prefers concise answers with direct implementation steps.",
    updatedAt: "2026-02-04",
  },
  {
    id: "mem-2",
    summary:
      "Primary stack is Next.js, TypeScript, shadcn/ui, and Bun runtime.",
    updatedAt: "2026-02-07",
  },
  {
    id: "mem-3",
    summary: "Default tone preference is practical, neutral, and technical.",
    updatedAt: "2026-02-10",
  },
];

export function PersonalizationSettingsPanel() {
  const [activeTab, setActiveTab] = useState("memories");
  const [memories, setMemories] = useState(DUMMY_MEMORIES);
  const [editingMemoryId, setEditingMemoryId] = useState<string | null>(null);
  const [memoryDraft, setMemoryDraft] = useState("");

  const [personalityMode, setPersonalityMode] = useState<"preset" | "custom">(
    "preset",
  );
  const [personalityPreset, setPersonalityPreset] = useState("balanced");
  const [customPersonality, setCustomPersonality] = useState("");

  const [toneMode, setToneMode] = useState<"preset" | "custom">("preset");
  const [tonePreset, setTonePreset] = useState("neutral");
  const [customTone, setCustomTone] = useState("");

  const canSaveMemory = useMemo(
    () => memoryDraft.trim().length > 0,
    [memoryDraft],
  );

  const startEditingMemory = (memory: MemoryItem) => {
    setEditingMemoryId(memory.id);
    setMemoryDraft(memory.summary);
  };

  const cancelEditingMemory = () => {
    setEditingMemoryId(null);
    setMemoryDraft("");
  };

  const saveMemory = (memoryId: string) => {
    if (!canSaveMemory) {
      return;
    }

    setMemories((current) =>
      current.map((memory) =>
        memory.id === memoryId
          ? {
              ...memory,
              summary: memoryDraft.trim(),
              updatedAt: new Date().toISOString().slice(0, 10),
            }
          : memory,
      ),
    );
    cancelEditingMemory();
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <BrainIcon className="size-5" />
          Personalization
        </CardTitle>
      </CardHeader>
      <CardContent>
        <Tabs className="gap-4" onValueChange={setActiveTab} value={activeTab}>
          <TabsList variant="default">
            <TabsTrigger value="memories">Memories</TabsTrigger>
            <TabsTrigger value="personality">Personality</TabsTrigger>
          </TabsList>

          <TabsContent className="space-y-3" value="memories">
            <p className="text-muted-foreground text-sm">
              These are memory summaries the agent extracted from prior chats.
            </p>

            {memories.map((memory) => {
              const isEditing = editingMemoryId === memory.id;

              return (
                <div className="rounded-lg border p-3" key={memory.id}>
                  <div className="mb-2 flex items-start justify-between gap-3">
                    <p className="text-muted-foreground text-xs">
                      Updated {memory.updatedAt}
                    </p>
                    {!isEditing ? (
                      <Button
                        onClick={() => startEditingMemory(memory)}
                        size="sm"
                        type="button"
                        variant="ghost"
                      >
                        <Edit3Icon className="size-4" />
                        Edit
                      </Button>
                    ) : null}
                  </div>

                  {isEditing ? (
                    <div className="space-y-2">
                      <Textarea
                        onChange={(event) => setMemoryDraft(event.target.value)}
                        rows={3}
                        value={memoryDraft}
                      />
                      <div className="flex items-center justify-end gap-2">
                        <Button
                          onClick={cancelEditingMemory}
                          size="sm"
                          type="button"
                          variant="outline"
                        >
                          <XIcon className="size-4" />
                          Cancel
                        </Button>
                        <Button
                          disabled={!canSaveMemory}
                          onClick={() => saveMemory(memory.id)}
                          size="sm"
                          type="button"
                        >
                          <SaveIcon className="size-4" />
                          Save
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <p className="text-sm">{memory.summary}</p>
                  )}
                </div>
              );
            })}
          </TabsContent>

          <TabsContent className="space-y-6" value="personality">
            <div className="space-y-3">
              <p className="font-medium text-sm">Agent personality</p>
              <div className="flex gap-2">
                <Button
                  onClick={() => setPersonalityMode("preset")}
                  size="sm"
                  type="button"
                  variant={personalityMode === "preset" ? "default" : "outline"}
                >
                  Preset
                </Button>
                <Button
                  onClick={() => setPersonalityMode("custom")}
                  size="sm"
                  type="button"
                  variant={personalityMode === "custom" ? "default" : "outline"}
                >
                  Custom
                </Button>
              </div>

              {personalityMode === "preset" ? (
                <Select
                  onValueChange={setPersonalityPreset}
                  value={personalityPreset}
                >
                  <SelectTrigger className="w-full sm:w-72">
                    <SelectValue placeholder="Select personality preset" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="balanced">Balanced Assistant</SelectItem>
                    <SelectItem value="teacher">Patient Teacher</SelectItem>
                    <SelectItem value="reviewer">Strict Reviewer</SelectItem>
                    <SelectItem value="builder">Fast Builder</SelectItem>
                  </SelectContent>
                </Select>
              ) : (
                <Textarea
                  onChange={(event) => setCustomPersonality(event.target.value)}
                  placeholder="Describe how your agent should behave..."
                  rows={4}
                  value={customPersonality}
                />
              )}
            </div>

            <div className="space-y-3">
              <p className="font-medium text-sm">Tone</p>
              <div className="flex gap-2">
                <Button
                  onClick={() => setToneMode("preset")}
                  size="sm"
                  type="button"
                  variant={toneMode === "preset" ? "default" : "outline"}
                >
                  Preset tones
                </Button>
                <Button
                  onClick={() => setToneMode("custom")}
                  size="sm"
                  type="button"
                  variant={toneMode === "custom" ? "default" : "outline"}
                >
                  Define tone
                </Button>
              </div>

              {toneMode === "preset" ? (
                <Select onValueChange={setTonePreset} value={tonePreset}>
                  <SelectTrigger className="w-full sm:w-72">
                    <SelectValue placeholder="Select tone" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="neutral">Neutral</SelectItem>
                    <SelectItem value="friendly">Friendly</SelectItem>
                    <SelectItem value="formal">Formal</SelectItem>
                    <SelectItem value="direct">Direct</SelectItem>
                    <SelectItem value="playful">Playful</SelectItem>
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  onChange={(event) => setCustomTone(event.target.value)}
                  placeholder="Explain the tone you want the agent to use"
                  value={customTone}
                />
              )}
            </div>

            <div className="flex justify-end">
              <Button type="button">Save personalization settings</Button>
            </div>
          </TabsContent>
        </Tabs>
      </CardContent>
    </Card>
  );
}
