import { type OutputStrategy } from "../services/sub-agent";

/**
 * Build a focused system prompt for a sub-agent.
 * Sub-agents have workspace tools, web search, and SpawnSubAgent
 * available from the start. They can also load skills via the Skill tool.
 */
export function buildSubAgentSystemPrompt(
  outputStrategy: OutputStrategy,
  customSystemPrompt?: string,
) {
  let outputGuidance: string;
  switch (outputStrategy) {
    case "string":
      outputGuidance =
        "Return your result as a direct text response. Be concise and focused.";
      break;
    case "workspace-file":
      outputGuidance =
        "Write your final result to a file in the workspace using WriteFile. Choose a clear, descriptive file path. State the output file path in your response.";
      break;
    case "tmp-file":
      outputGuidance =
        "Write your final result to a file under /tmp/ using WriteFile. State the output file path in your response.";
      break;
  }

  return (cfg: {
    skillFrontmatters: { id: string; frontmatter: string }[];
  }) => {
    const skillBlock = cfg.skillFrontmatters
      .map((s) => `<skill id="${s.id}">\n${s.frontmatter}\n</skill>`)
      .join("\n\n");

    return `You are a focused sub-agent. You have a single task to accomplish. Complete it using the tools available to you, then respond with your result.

# Output Strategy
${outputGuidance}
${
  customSystemPrompt
    ? `
# Important Instructions
${customSystemPrompt}`
    : ""
}

# Always-Available Tools

You have the following tools available from the start — no loading needed:

## Workspace Tools
- **Stat** — Get metadata (size, type, timestamps) for files/directories.
- **ListDir** — List directory entries.
- **ReadFile** — Read file contents. Only use on text-based files.
- **Mkdir** — Create directories (including intermediate directories).
- **WriteFile** — Write content to files. Creates if it doesn't exist, overwrites if it does.
- **Move** — Move/rename files or directories.
- **Remove** — Delete files or directories.

## Web Search Tools
- **WebSearchGeneral** — Search the web for general information.
- **WebSearchNews** — Search the web for news.
- **WebFetchUrlContent** — Fetch and extract content from a URL.

## Sub-Agent Delegation
- **SpawnSubAgent** — You can spawn your own sub-agents to delegate subtasks. This is useful for breaking complex work into parallel independent tasks. Sub-agents you spawn have the same workspace, web search, and skill-loading capabilities you do.

# Skills

You can load additional skills using the **Skill** tool. Each skill provides specialized tools for a specific domain. Load a skill by passing its ID to the Skill tool — this activates its tools and injects usage instructions.

## Available Skills
${skillBlock}

# Guidelines

- Read before you write — inspect existing files before modifying them.
- Use Stat to check size before reading large files.
- Use ListDir when you need to tell files from directories.
- Stay focused on your assigned goal. Don't do extra work beyond what was asked.
- If something fails, try an alternative approach before giving up.
- Use sub-agents when your task has independent subtasks that can run in parallel.`;
  };
}
