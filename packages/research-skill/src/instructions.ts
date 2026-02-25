export const description =
  "Structured multi-phase research skill. Emphasizes coordinator-driven sub-agent orchestration: scoped task packets, parallel fan-out, hierarchical delegation, structured artifacts, quality gates, and evidence-first final synthesis using existing search/fetch/sub-agent tools.";

export const instructions = `
# Research Skill
You have access to a structured research methodology. This skill provides NO additional tools — it teaches you how to use your existing tools (WebSearchGeneral, WebSearchNews, WebFetchUrlContent, SpawnSubAgent, and workspace tools) in a disciplined, sub-agent-first workflow.

Follow these phases sequentially. Do not skip phases.

---

## Operating Model (Coordinator + Workers)
You are the **coordinator**. Sub-agents are **workers**.

Workers are fully capable agents. They have:
- All workspace tools (Stat, ListDir, ReadFile, Mkdir, WriteFile, Move, Remove)
- All web search tools (WebSearchGeneral, WebSearchNews, WebFetchUrlContent)
- The Skill tool — they can load any skill (e.g., docx-skill for document analysis)
- **SpawnSubAgent** — workers can spawn their own sub-agents (up to 3 levels deep total). This enables hierarchical delegation: you spawn a coordinator-worker, and it fans out to its own workers.

### Coordinator responsibilities:
- Decompose the problem into independent tasks.
- Assign each worker a single narrow objective with non-overlapping scope.
- Enforce structured outputs and file paths.
- Validate, deduplicate, and synthesize worker artifacts.
- Decide when a task is complex enough to warrant a hierarchical worker (one that delegates further).

### Worker responsibilities:
- Execute only assigned scope.
- Write structured output to the exact target file.
- Include evidence and uncertainty explicitly.
- Do not perform final synthesis across all sources.
- For complex subtasks, spawn their own sub-agents to parallelize the work.

**Default rule:** Prefer spawning workers for research work. Coordinator does orchestration and quality control, not bulk search/extraction.

---

## Phase 1 — Clarify Intent & Constraints
Before searching, understand what the user actually needs.

1. Analyze the user's research request carefully.
2. Ask 2–5 clarification questions to narrow:
   - **Scope**: How broad or narrow? (e.g., "all renewable energy" vs. "solar panel efficiency trends in 2024–2025")
   - **Audience**: Who is this for? (executive summary, technical deep-dive, academic, casual?)
   - **Depth**: Surface overview or exhaustive analysis?
   - **Recency**: Does this need the latest data, or is historical context fine?
   - **Output format**: Report, bullet points, comparison table, slide-ready?
   - **Decision use**: What decision should this research inform?
3. Wait for the user's answers before proceeding. If needed further converse with them to properly understand the research scope and requirements.

---

## Phase 2 — Plan, Scope, and Artifact Layout
Create a research plan and persist it.

1. Generate a short kebab-case name for this research (e.g., "solar-efficiency-trends").
2. Create a user-visible working directory in the workspace with collision resistance: \`research/[name]-[short-hash]/\`
3. Create:
   - \`plan.md\`
   - \`task-board.md\` (task IDs, owner worker, status)
   - \`searches/\`
   - \`summaries/\`
   - \`evidence/\` (optional extracted tables/data points)
4. Write \`plan.md\` containing:
   - **Topic**: One-sentence statement of the research question
   - **Scope**: Boundaries — what's included and excluded
   - **Research angles**: 5–10 distinct angles (market, technical, regulatory, critical viewpoints, etc.)
   - **Target queries**: 8–15 distinct queries mapped to angles
   - **Delegation strategy**: Which tasks are simple (flat workers) vs. complex (hierarchical workers that should fan out further)
   - **Output format**: What the final deliverable looks like
   - **Success criteria**: What makes the answer "good enough" for the user
5. Share the plan with the user and confirm before proceeding.

---

## Phase 3 — Broad Discovery (parallel worker fan-out)
Run at least 5 parallel discovery tasks to gather diverse sources.

1. Create 5–10 discovery tasks from research angles. Each task must be meaningfully distinct.
2. Spawn workers in parallel using \`SpawnSubAgent\` (use file output mode so artifacts are persisted to disk).
3. For each worker, provide a **task packet** with:
   - \`task_id\`
   - objective and angle
   - allowed tools (\`WebSearchGeneral\`, \`WebSearchNews\`)
   - 2–4 target queries however the amount of searches and queries to use should be determined by the worker.
   - stop condition (e.g., "return top 10 candidate sources") that said encourage the worker to search more for finding meaningful results
   - required output file path
4. Each worker writes to \`research/[name]-[short-hash]/searches/task-[id].md\`
5. After completion, coordinator validates outputs:
   - file exists and follows required format
   - URLs are present and not obvious duplicates
   - each task contributed unique value
6. If a worker fails/returns weak output, respawn once with a narrower query packet.
7. Build a master source list with preliminary relevance scores (high/medium/low).

### Hierarchical discovery (for broad topics)

When a research angle is itself broad (e.g., "regulatory landscape across 10 countries"), spawn a **coordinator-worker** instead of a flat worker. In its goal, instruct it to:
- Break its angle into sub-angles
- Spawn its own workers for each sub-angle in parallel
- Consolidate sub-worker outputs into a single structured file

This lets you cover broad topics deeply without bottlenecking on a single worker.

**Sub-agent goal template (flat worker):**
\`\`\`
Task ID: [id]
Objective: Investigate [specific angle] for [topic]
Queries:
- [query 1]
- [query 2]
Use WebSearchGeneral/WebSearchNews only.
Return:
1) 3-6 key findings (bullets, factual, non-redundant)
2) 3-5 candidate URLs with one-line rationale each
3) A confidence note (high/medium/low) and known blind spots
Write to: research/[name]-[short-hash]/searches/task-[id].md
\`\`\`

**Sub-agent goal template (coordinator-worker for broad angles):**
\`\`\`
Task ID: [id]
Objective: You are a research coordinator for [broad angle] within [topic].
Break this angle into 3-5 distinct sub-angles.
For each sub-angle, spawn a sub-agent worker to search and gather sources.
Each sub-agent should:
- Use WebSearchGeneral/WebSearchNews
- Return 3-5 key findings and 3-5 candidate URLs
- Write results to research/[name]-[short-hash]/searches/task-[id]-sub-[n].md
After all sub-agents complete, read their outputs and consolidate into a single summary.
Write the consolidated summary to: research/[name]-[short-hash]/searches/task-[id].md
\`\`\`

---

## Phase 4 — Deep Extraction (parallel worker fan-out)
Extract detailed information from the best sources found in Phase 3.

1. Collect all URLs from the discovery task files. Deduplicate and rank by relevance.
2. Select the top 8–15 URLs for deep extraction.
3. Partition URLs so each worker owns 1–3 URLs with no overlap.
4. Spawn workers in parallel. Each worker must:
   - Use \`WebFetchUrlContent\` to retrieve full page content
   - Extract key claims, data points, assumptions, and limitations
   - Capture concrete evidence: numbers, dates, names, and short quotes
   - Tag each claim with confidence: high/medium/low
   - Note contradictions relative to other high-priority sources (if known)
   - Use file output mode so results are written to disk
5. Each worker writes to \`research/[name]-[short-hash]/summaries/source-[id].md\`
6. Coordinator validates each summary before synthesis:
   - includes source URL and publication/update date if available
   - includes at least 3 concrete evidence points
   - separates facts from interpretation
7. Re-run extraction tasks that fail validation or miss required evidence.

### Deep-dive extraction (for dense or complex sources)

When a source is particularly dense (long technical papers, comprehensive reports), spawn a **coordinator-worker** that:
- Fetches the content itself
- Breaks the source into logical sections
- Spawns sub-workers to extract and summarize each section in parallel
- Consolidates section summaries into a single structured source summary

**Sub-agent goal template (flat worker):**
\`\`\`
Task ID: [id]
Fetch and deeply analyze these URLs: [url1, url2, ...]
For each URL:
- Extract key facts, data points, and arguments
- Note specific numbers, dates, names, and direct quotes
- Label each key claim with confidence (high/medium/low)
- Flag contradictions, caveats, or missing context
Write a dense summary to research/[name]-[short-hash]/summaries/source-[id].md
\`\`\`

**Sub-agent goal template (coordinator-worker for dense sources):**
\`\`\`
Task ID: [id]
You are an extraction coordinator for this source: [url]
1. Fetch the full content using WebFetchUrlContent
2. Identify the major sections/topics in the content
3. For each section, spawn a sub-agent to extract and summarize:
   - Key facts, data points, and arguments
   - Specific numbers, dates, names, and direct quotes
   - Confidence labels (high/medium/low) on claims
   - Each sub-agent writes to research/[name]-[short-hash]/summaries/source-[id]-section-[n].md
4. After all sub-agents complete, read their outputs and consolidate into a single comprehensive summary
Write consolidated summary to: research/[name]-[short-hash]/summaries/source-[id].md
\`\`\`

---

## Phase 5 — Final Report
Synthesize all extracted information into a comprehensive deliverable.

1. Read all files in \`research/[name]-[short-hash]/summaries/\`
2. Build an evidence matrix mapping claims -> supporting/conflicting sources.
3. Cross-reference findings across sources to identify:
   - Consensus points (multiple sources agree)
   - Contradictions or debates
   - Data gaps or areas needing further research
4. Write a comprehensive final report that includes:
   - **Executive Summary**: 3–5 sentence overview
   - **Key Findings**: Organized by theme or question, not by source
   - **Key Writeups**: For each major element of the research write:
     - **Detailed Analysis**: In-depth discussion with supporting evidence
     - **Contradictions & Caveats**: Where sources disagree or data is limited
     - **Sources**: Numbered citation list with URLs
   - **Confidence Summary**: What is well-supported vs tentative
5. Write the final report to the user's workspace as a visible deliverable.

---

## Coordinator Rules (Critical)
- **Always fan out**: Discovery and extraction should be worker-driven and parallelized.
- **Use hierarchical delegation for breadth**: When an angle or source is too broad for a single worker, spawn a coordinator-worker that delegates further. Don't overload flat workers with complex multi-part tasks.
- **Single-owner tasks**: One worker per task packet; avoid overlapping ownership.
- **Structured artifacts only**: Workers must write to required paths with consistent section headers.
- **Validation gate**: Coordinator must validate worker outputs before synthesis.
- **Retry policy**: Failed/weak tasks get one targeted retry.
- **Persist intermediate work**: Write all intermediate artifacts in \`research/[name]-[short-hash]/\` so the user can inspect progress live.
- **Expose progress paths**: In status updates, reference the active workspace artifact paths being produced.
- **Dense evidence**: Retain concrete facts; remove narrative fluff.
- **Cite everything**: Every major claim maps to at least one source URL.
- **Ask before proceeding**: Confirm plan (Phase 2) before expensive fan-out.
- **Adapt scope**: If discovery changes scope materially, update plan and task-board before continuing.
- **Depth budget**: Sub-agents can nest up to 3 levels deep. Design delegation to stay within this limit (you → coordinator-worker → leaf workers = 3 levels).
`.trim();
