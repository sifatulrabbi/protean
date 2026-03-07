---
project_name: Protean
description: AI Agent for everyday use cases with hosting flexibility
version: 2.0.0
language: TypeScript. Bun is the runtime
repo_structure: monorepo
auth_provider: WorkOS
---

# Primary applications

1. Next.js Web App deployed on vercel
2. Sandbox Client -- built with Dockerode and bun for serving the sandbox. Is also self host-able by the enthusiastic users

# Philosophy and core goals

Today the LLMs are much more capable and a proper AI Agent system requires a bash + OS to perform tasks on behalf of the user. For example, researching with multiple sub-agents + writing multiple Markdown files to gather information on various topics, installing packages then using them to convert or perform things asked by the user such as writing DOCX files with Docx.js code (first write the code in a "file".ts then install the docx.js with bun finally running the script with bun ./"file".ts).

The sandbox system either allows the use to use a hosted sandbox that is only for themselves and is never shared with anyone else. Or use a self-host-able service that would connect their local files and directories with Protean agent so that the agent can help the user get things done with their local files.

# Structure of the system and some Pseudo Code

The following would include some pseudo code and explanation of some of the core system components but does not promise to include everything necessary.

```ts
// The sandbox client is the client that uses any sandbox either a remote one or
// a self-hosted one.
interface ISandboxClient {
  /**
   * Will automatically set to false if the connection with the underlying
   * sandbox fails. This is being ensured by a heartbeat implementation with the
   * sandbox server. E.g., ping every 5 second.
   */
  isConnected: boolean;
  /**
   * Internal state of the ISandboxClient indicating any error in the connection
   */
  connErr: Error | null;
  bash: IBash;
  fs: IFilesystem; // The file-system exposed by the sandbox service/server
}

interface IBash {
  /**
   * Use scope to scope the command within a valid path in the sandbox.
   */
  exec: (
    cmd: string,
    scope?: string,
  ) => Promise<{
    result: { stdout: string; stderr: string };
    error: Error | null;
  }>;
}

interface IFilesystem {
  readFile: (
    fullPath: string,
    range: Range,
  ) => Promise<{
    fullPath: string;
    content: string;
    range: Range;
    error: Error | null;
  }>;
  listDir: (
    fullPath: string,
  ) => Promise<{ fullPath: string; entries: Entity[]; error: Error | null }>;
  stat: (fullPath: string) => Promise<{
    fullPath: string;
    entity: Entity | null;
    error: Error | null;
  }>;
  writeFile: (
    fullPath: string,
    content: string,
  ) => Promise<{ fullPath: string; error: Error | null }>;
  create: (
    fullPath: string,
    entityType: "file" | "directory",
    opts?: { recursive?: boolean },
  ) => Promise<{
    fullPath: string;
    entity: Entity;
    error: Error | null;
  }>;
  copy: (
    sourceFullPath: string,
    copyToFullPath: string,
  ) => Promise<{
    sourceFullPath: string;
    copyToFullPath: string;
    error: Error | null;
  }>;
  move: (
    sourceFullPath: string,
    newFullPath: string,
    opts?: { recursive?: boolean },
  ) => Promise<{
    sourceFullPath: string;
    newFullPath: string;
    error: Error | null;
  }>;
  remove: (
    fullPath: string,
    opts?: { recursive?: boolean },
  ) => Promise<{
    fullPath: string;
    error: Error | null;
  }>;
}
```

## Concept of Project

The concept of the agent working in a dir is called in the domain of Protean a "Project". Project is just a scoped FS access layer to ensure what we are allowing the agent to see or work with in a given moment.

A project is a directory in the sandboxed workspace's `/workspace/projects/*`. A project's name is directly the name of the directory in the path. This allows us to not complicate the concept of project too much.

For when we are letting users connect their local directories to work with protean we are allowing users to create a mount of their local directory in our sandboxed workspace's `/workspace/projects/*` path. E.g., choosing my `/Users/sifatul/coding/protean/` as the working project means the system is mounting it in `/workspace/projects/protean:/Users/sifatul/coding/protean/`.

```ts
interface ProjectFactory {
  (cfg: {
    sandboxClient: ISandboxClient;
    projectPath: string;
    mnt: IMount;
  }): Promise<IProject>;
}

interface IProject {
  /** Project scoped IFilesystem managed by the project object */
  fs: IFilesystem;
  /** Project scoped IBash managed by the project object */
  bash: IBash;
}
```

### Mounting device dir to remote sandbox

```ts
interface IMount extends IFilesystem {
  id: string;
  workspacePath: string;
}
```

Mounting local user files to a project is to create a link with the user's local files. We will have a lightweight sandbox like host app running in the user end so that our WebApp can use the host. But we need to also think how best can we setup the host app and if we should expose the host app from user's device or use the WebApp to resolve all the requests by letting talk privately with the host app?
