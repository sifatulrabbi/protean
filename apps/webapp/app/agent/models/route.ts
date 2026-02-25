import {
  getDefaultModelSelection,
  getModelCatalog,
} from "@protean/model-catalog";

export async function GET() {
  return Response.json(
    {
      defaultSelection: getDefaultModelSelection(),
      providers: getModelCatalog(),
    },
    { status: 200 },
  );
}
