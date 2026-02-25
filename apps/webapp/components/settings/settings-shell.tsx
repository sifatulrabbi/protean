"use client";

import Link from "next/link";
import { useState } from "react";
import { ArrowLeftIcon, MenuIcon } from "lucide-react";
import { PersonalizationSettingsPanel } from "@/components/settings/personalization-settings-panel";
import { UsageSettingsPanel } from "@/components/settings/usage-settings-panel";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

const SETTINGS_TABS = [
  { label: "Usage", value: "usage" },
  { label: "Personalization", value: "personalization" },
  { label: "Capabilities", value: "capabilities" },
  { label: "Options", value: "options" },
] as const;

function SettingsNav({
  onTabSelect,
}: {
  onTabSelect?: (tabValue: string) => void;
}) {
  return (
    <>
      <Button
        asChild
        className="w-full justify-start"
        size="sm"
        variant="outline"
      >
        <Link href="/chats/new">
          <ArrowLeftIcon className="size-4" />
          Back to chats
        </Link>
      </Button>

      <TabsList
        className="mt-3 flex w-full flex-col items-stretch gap-1 bg-transparent p-0"
        variant="line"
      >
        {SETTINGS_TABS.map((tab) => (
          <TabsTrigger
            className="h-10 justify-start rounded-lg border border-transparent px-3 font-medium text-muted-foreground text-sm transition-all hover:bg-accent hover:text-foreground data-[state=active]:bg-accent data-[state=active]:text-foreground data-[state=active]:shadow-none"
            key={tab.value}
            onClick={() => onTabSelect?.(tab.value)}
            value={tab.value}
          >
            {tab.label}
          </TabsTrigger>
        ))}
      </TabsList>
    </>
  );
}

export function SettingsShell() {
  const [activeTab, setActiveTab] = useState<string>(SETTINGS_TABS[0].value);
  const [mobileOpen, setMobileOpen] = useState(false);

  const handleTabChange = (value: string) => {
    setActiveTab(value);
    setMobileOpen(false);
  };

  return (
    <Tabs
      className="flex min-h-screen"
      onValueChange={handleTabChange}
      orientation="vertical"
      value={activeTab}
    >
      <aside className="hidden h-screen w-72 flex-col border-r border-sidebar-border bg-sidebar p-3 md:flex">
        <SettingsNav />
      </aside>

      <div className="fixed top-3 left-3 z-40 md:hidden">
        <Sheet onOpenChange={setMobileOpen} open={mobileOpen}>
          <SheetTrigger asChild>
            <Button size="icon" variant="outline">
              <MenuIcon className="size-5" />
              <span className="sr-only">Open settings navigation</span>
            </Button>
          </SheetTrigger>
          <SheetContent
            side="left"
            showCloseButton={false}
            className="flex w-72 flex-col bg-sidebar p-3"
          >
            <SheetHeader className="p-0">
              <SheetTitle className="sr-only">Settings navigation</SheetTitle>
            </SheetHeader>
            <SettingsNav onTabSelect={handleTabChange} />
          </SheetContent>
        </Sheet>
      </div>

      <main className="min-w-0 flex-1">
        <div className="flex h-screen min-h-0 flex-col bg-background">
          <div className="mx-auto flex min-h-0 w-full max-w-4xl flex-1 flex-col px-3 pt-14 pb-4 md:px-4 md:pt-6">
            {SETTINGS_TABS.map((tab) => (
              <TabsContent className="mt-0" key={tab.value} value={tab.value}>
                {tab.value === "usage" ? <UsageSettingsPanel /> : null}
                {tab.value === "personalization" ? (
                  <PersonalizationSettingsPanel />
                ) : null}
                {tab.value === "capabilities" ? (
                  <Card>
                    <CardHeader>
                      <CardTitle>Capabilities</CardTitle>
                    </CardHeader>
                    <CardContent />
                  </Card>
                ) : null}
                {tab.value === "options" ? (
                  <Card>
                    <CardHeader>
                      <CardTitle>Options</CardTitle>
                    </CardHeader>
                    <CardContent />
                  </Card>
                ) : null}
              </TabsContent>
            ))}
          </div>
        </div>
      </main>
    </Tabs>
  );
}
