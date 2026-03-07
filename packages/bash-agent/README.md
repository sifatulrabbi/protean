# Bash Agent

`@protean/bash-agent` is a workspace-root-bounded agent package built on Vercel AI SDK `ToolLoopAgent`.

It:

- loads an existing thread from `AgentMemory`
- resolves the thread's persisted OpenRouter model selection
- exposes filesystem and shell tools that cannot escape the configured workspace root
- runs the agent against the thread's active history
- persists the assistant reply back into thread memory

## Safety model

The v1 boundary is intentionally simple:

- every filesystem path is resolved inside the configured `workspaceRoot`
- every shell command runs with a validated `cwd` under that root
- shell execution is bounded by timeout and output caps
- only explicitly allowlisted environment keys are passed through, plus minimal shell runtime keys

This is not a semantic command sandbox. The shell can still execute arbitrary commands inside the workspace boundary.

## Minimal usage

```ts
import { createBashAgent } from "@protean/bash-agent";

// The caller persists the latest user message first.
const bashAgent = await createBashAgent({
  threadId,
  memory,
  workspaceRoot: "/absolute/path/to/workspace",
});

const result = await bashAgent.generateThread();

console.log(result.text);
```
