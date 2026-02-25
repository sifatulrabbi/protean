import { describe, expect, it } from "bun:test";
import catalogJson from "../data/models.catalog.json";
import {
  getModelCatalog,
  getDefaultModelSelection,
  findModel,
} from "../catalog-parser";

interface RawOpenRouterModel {
  id: string;
  context_length: number;
  top_provider?: {
    context_length?: number | null;
    max_completion_tokens?: number | null;
  } | null;
}

function getTotalContext(model: RawOpenRouterModel): number {
  return typeof model.top_provider?.context_length === "number" &&
    model.top_provider.context_length > 0
    ? model.top_provider.context_length
    : model.context_length;
}

function getRawMaxOutput(model: RawOpenRouterModel): number {
  const total = getTotalContext(model);
  const reportedMaxOutput = model.top_provider?.max_completion_tokens;

  return typeof reportedMaxOutput === "number" && reportedMaxOutput > 0
    ? Math.min(total, reportedMaxOutput)
    : total;
}

const rawCatalog = catalogJson as Record<string, RawOpenRouterModel[]>;

describe("getModelCatalog", () => {
  it("returns a non-empty list of providers", () => {
    const providers = getModelCatalog();
    expect(providers.length).toBeGreaterThan(0);
  });

  it("each provider has an id, name, and at least one model", () => {
    const providers = getModelCatalog();
    for (const provider of providers) {
      expect(provider.id).toBeTruthy();
      expect(provider.name).toBeTruthy();
      expect(provider.models.length).toBeGreaterThan(0);
    }
  });
});

describe("getDefaultModelSelection", () => {
  it("returns a valid model selection", () => {
    const selection = getDefaultModelSelection();
    expect(selection.providerId).toBeTruthy();
    expect(selection.modelId).toBeTruthy();
    expect(["none", "low", "medium", "high"]).toContain(
      selection.reasoningBudget,
    );
  });

  it("default model exists in the catalog", () => {
    const selection = getDefaultModelSelection();
    const model = findModel(selection.providerId, selection.modelId);
    expect(model).toBeDefined();
  });
});

describe("findModel", () => {
  it("finds a model that exists", () => {
    const selection = getDefaultModelSelection();
    const model = findModel(selection.providerId, selection.modelId);
    expect(model).toBeDefined();
    expect(model!.id).toBe(selection.modelId);
  });

  it("returns undefined for non-existent model", () => {
    const model = findModel("no-such-provider", "no-such-model");
    expect(model).toBeUndefined();
  });

  it("returns undefined for existing provider but wrong model", () => {
    const selection = getDefaultModelSelection();
    const model = findModel(selection.providerId, "nonexistent-model-id");
    expect(model).toBeUndefined();
  });

  it("normalizes near-full output windows to 40% of total context", () => {
    const nearFullEntries = Object.entries(rawCatalog)
      .flatMap(([providerId, models]) =>
        models.map((model) => ({ providerId, model })),
      )
      .filter(
        ({ model }) => getRawMaxOutput(model) / getTotalContext(model) >= 0.9,
      );

    expect(nearFullEntries.length).toBeGreaterThan(0);

    for (const entry of nearFullEntries) {
      const parsedModel = findModel(entry.providerId, entry.model.id);
      expect(parsedModel).toBeDefined();

      const total = getTotalContext(entry.model);
      const expectedMaxOutput = Math.max(1, Math.floor(total * 0.4));

      expect(parsedModel!.contextLimits.total).toBe(total);
      expect(parsedModel!.contextLimits.maxOutput).toBe(expectedMaxOutput);
      expect(parsedModel!.contextLimits.maxInput).toBe(
        total - expectedMaxOutput,
      );
    }
  });

  it("keeps constrained output windows unchanged", () => {
    const constrainedEntry = Object.entries(rawCatalog)
      .flatMap(([providerId, models]) =>
        models.map((model) => ({ providerId, model })),
      )
      .find(({ model }) => {
        const reportedMaxOutput = model.top_provider?.max_completion_tokens;
        if (!(typeof reportedMaxOutput === "number" && reportedMaxOutput > 0)) {
          return false;
        }

        return getRawMaxOutput(model) / getTotalContext(model) < 0.9;
      });

    expect(constrainedEntry).toBeDefined();
    if (!constrainedEntry) {
      return;
    }

    const parsedModel = findModel(
      constrainedEntry.providerId,
      constrainedEntry.model.id,
    );
    expect(parsedModel).toBeDefined();

    const total = getTotalContext(constrainedEntry.model);
    const expectedMaxOutput = getRawMaxOutput(constrainedEntry.model);

    expect(parsedModel!.contextLimits.total).toBe(total);
    expect(parsedModel!.contextLimits.maxOutput).toBe(expectedMaxOutput);
    expect(parsedModel!.contextLimits.maxInput).toBe(total - expectedMaxOutput);
  });
});
