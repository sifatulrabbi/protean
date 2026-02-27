# Protean repo guidelines

- The repository uses bun for all the typescript apps and packages.
- Always run the unit tests after a unit of work to ensure you are not regressing.
- However, skip running the heavy duty integration tests unless its necessary.
- When working in the apps/webapp/ always think first of choose the right skill to load.
  - When it's frontend work choose the "frontend-skill"
    - For the frontend work prefer using "shadcn-ui" skill and shadcn ui over manual work.
    - However, the frontend parts that are related to the AI chat use "ai-elements" skill and ai-elements over manual work.
  - When asked for reviews and analysis of the code for finding improvement areas use the "vercel-react-best-practices" with
