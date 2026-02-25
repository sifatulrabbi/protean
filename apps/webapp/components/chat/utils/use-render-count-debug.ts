"use client";

import { useEffect } from "react";

const renderCounts = new Map<string, number>();

export function useRenderCountDebug(componentName: string): void {
  useEffect(() => {
    if (process.env.NODE_ENV !== "development") {
      return;
    }

    const nextCount = (renderCounts.get(componentName) ?? 0) + 1;
    renderCounts.set(componentName, nextCount);

    // Dev-only sanity check for refactor-induced rerender regressions.
    console.debug(`[render-count] ${componentName}: ${nextCount}`);
  });
}
