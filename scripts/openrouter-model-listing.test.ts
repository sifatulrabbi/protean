async function test() {
  const res = await fetch("https://openrouter.ai/api/v1/models", {
    headers: {
      // The API requires an auth token – use the key you got from openrouter.ai
      Authorization: `Bearer ${process.env.OPENROUTER_API_KEY}`,
      // Explicit JSON request/response
      Accept: "application/json",
    },
  });
  if (!res.ok) {
    throw new Error(
      `❗ OpenRouter request failed: ${res.status} ${res.statusText}`,
    );
  }
  const data = await res.json(); // shape: { data: OpenRouterModel[] }
  const f = Bun.file("./openrouter-models.json");
  await f.write(JSON.stringify(data.data, undefined, 2));

  console.log(data.data.length);
}
void test();
