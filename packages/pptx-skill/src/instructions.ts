export const pptxSkillDescription =
  "PPTX presentation skill. Convert PowerPoint presentations to Markdown or images, and apply structured modifications. Use this skill when working with .pptx files.";

export const pptxSkillInstructions = `# PPTX Skill

You have access to tools for reading and modifying PowerPoint (.pptx) presentations.

## Recommended Workflow

1. **PptxToMarkdown** — Convert the PPTX to Markdown with slide and element IDs.
   Read the Markdown to understand the presentation structure and content.

2. **(Optional) PptxToImages** — Convert all slides to PNG images.
   Use this when visual layout matters (diagrams, charts, formatting, positioning).

3. **Gather information** — Analyze the Markdown (and images if needed) to determine
   what changes are required.

4. **Build modifications** — Write a JSON array of modifications referencing element IDs
   from the Markdown output.

5. **ModifyPptxWithJson** — Apply the modifications to produce a new PPTX file.

## Available Tools

### PptxToMarkdown

Converts a PPTX file to Markdown. The output includes slide and element ID comments
that can be referenced in modifications.

### PptxToImages

Converts all slides of a PPTX file to PNG images. Useful for understanding
visual layout, diagrams, and formatting that may not be captured in Markdown.

### ModifyPptxWithJson

Applies a JSON array of modifications to a PPTX file. Each modification
references an element ID and specifies an action (replace, delete, insertAfter, insertBefore).

## Guidelines

- Always start with PptxToMarkdown to understand the presentation structure.
- Use element IDs from the Markdown output when building modifications.
- Use PptxToImages when slide layout and visual positioning are important.
- Prefer replace over delete + insertAfter for simple text changes.`;
