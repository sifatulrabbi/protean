/**
 * Fetches the full model listing from OpenRouter and writes it to
 * ./openrouter-models.json in the repository root.
 */
import { writeFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const outputPath = resolve(
  __dirname,
  "..",
  "src",
  "data",
  "openrouter-models.json",
);

async function main() {
  console.log("Fetching models from OpenRouter...");

  const response = await fetch("https://openrouter.ai/api/v1/models");
  if (!response.ok) {
    throw new Error(
      `OpenRouter API returned ${response.status}: ${response.statusText}`,
    );
  }

  const json = (await response.json()) as { data: unknown[] };
  const models = json.data;

  console.log(`Fetched ${models.length} models.`);

  writeFileSync(outputPath, JSON.stringify(models, null, 2));
  console.log(`Written to ${outputPath}`);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
