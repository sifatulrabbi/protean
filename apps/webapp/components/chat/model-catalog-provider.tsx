"use client";

import { createContext, use, type ReactNode } from "react";
import type { AIModelProviderEntry } from "@protean/model-catalog";

const ModelCatalogContext = createContext<AIModelProviderEntry[] | null>(null);

export function ModelCatalogProvider({
  children,
  providers,
}: {
  children: ReactNode;
  providers: AIModelProviderEntry[];
}) {
  return (
    <ModelCatalogContext value={providers}>{children}</ModelCatalogContext>
  );
}

export function useModelCatalog(): AIModelProviderEntry[] {
  const ctx = use(ModelCatalogContext);
  if (!ctx) {
    throw new Error("useModelCatalog must be used within ModelCatalogProvider");
  }
  return ctx;
}
