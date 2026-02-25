/**
 * Reads openrouter-models.json and groups models by provider,
 * writing the result to src/data/models.catalog.json.
 *
 * Direct TypeScript port of scripts/convert_model_listing.go.
 */
import { readFileSync, writeFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

interface OpenRouterModel {
  id: string;
  name: string;
  context_length: number;
  top_provider?: {
    context_length?: number;
    max_completion_tokens?: number;
    is_moderated?: boolean;
  } | null;
  pricing?: {
    prompt?: string;
    completion?: string;
  };
  supported_parameters?: string[];
  [key: string]: unknown;
}

const __dirname = dirname(fileURLToPath(import.meta.url));
const inputPath = resolve(
  __dirname,
  "..",
  "src",
  "data",
  "openrouter-models.json",
);
const outputPath = resolve(
  __dirname,
  "..",
  "src",
  "data",
  "models.catalog.json",
);

function main() {
  const raw = readFileSync(inputPath, "utf-8");
  const models: OpenRouterModel[] = JSON.parse(raw);

  const providerOrder: string[] = [];
  const providerModels = new Map<string, OpenRouterModel[]>();

  for (const model of models) {
    const parts = model.id.split("/");
    if (parts.length < 2) {
      continue;
    }

    const providerId = parts[0]!;
    const existing = providerModels.get(providerId);
    if (existing) {
      existing.push(model);
    } else {
      providerOrder.push(providerId);
      providerModels.set(providerId, [model]);
    }
  }

  console.log(`Total models found: ${models.length}`);
  console.log(`Providers: ${providerOrder.join(", ")}`);

  const result: Record<string, OpenRouterModel[]> = {};
  for (const providerId of providerOrder) {
    result[providerId] = providerModels.get(providerId)!;
  }

  writeFileSync(outputPath, JSON.stringify(result, undefined, 2));
  console.log(`Written to ${outputPath}`);
}

main();
