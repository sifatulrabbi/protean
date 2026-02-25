export const xlsxSkillDescription =
  "XLSX spreadsheet skill. Convert Excel spreadsheets to JSONL preserving formulas, and apply structured modifications. Use this skill when working with .xlsx files.";

export const xlsxSkillInstructions = `# XLSX Skill

You have access to tools for reading and modifying Excel (.xlsx) spreadsheets.

## Recommended Workflow

1. **XlsxToJsonl** — Convert the XLSX to JSONL format preserving formulas.
   Read the JSONL output to understand sheet structure, cell values, and formulas.

2. **Gather information** — Analyze the JSONL output to determine what changes are required.

3. **Build modifications** — Write a JSONL array of modifications specifying sheet, cell,
   value, and optional formula for each change.

4. **ModifyXlsxWithJsonl** — Apply the modifications to produce a new XLSX file.

## Available Tools

### XlsxToJsonl

Converts an XLSX file to JSONL format. Each sheet is output as a separate .jsonl file
with cell references, values, and formulas preserved.

### ModifyXlsxWithJsonl

Applies JSONL modifications to an XLSX file. Each modification specifies a sheet name,
cell reference (e.g., "A1"), new value, and optional formula.

## Guidelines

- Always start with XlsxToJsonl to understand the spreadsheet structure.
- Preserve formulas when possible — use the formula field in modifications.
- Reference cells using standard Excel notation (e.g., "A1", "B2", "AA10").
- Specify the exact sheet name when making modifications.`;
