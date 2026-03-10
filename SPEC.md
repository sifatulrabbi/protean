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
  // ... More fields.
  // I'm just showing an example and not claiming this is literally what we should have
}
```

Mounting local user files to a project is to create a link with the user's local files. We will have a lightweight sandbox like host app running in the user end so that our WebApp can use the host. But we need to also think how best can we setup the host app and if we should expose the host app from user's device or use the WebApp to resolve all the requests by letting talk privately with the host app?

## Sandbox concept

The sandbox is just a docker container per user in the host machine of ours. Where we'll mount a directory per user in the path `/workspace`. Every file system operation from the Docker container (the sandbox) needs to be done on the attached volume or mounted directory. What I mean is:

- Inside the Docker container if the user is using the "remote storage" option then we'll mount a directory for the user from host machine e.g. `/home/hostmachine/data/protean/users/user-id` to the `/workspace` path in the container. This will be as simple as creating the user specific directory in the host-machine then attaching that as a volume to the docker container.

- However, the tough one will be mounting a user's local directory to the container as a volume. For that we may need to write some kind of FUSE or directory mounting logic. Since we are not wanting the user's to do complex things like setting up SSH we need to figure out a way to be able to use some strategy and mount the user's local directory to the remote container of ours. One path I see is this:
  - Mount the user's selected directory as a project to the remote host-machine of ours. E.g., user's local directory `/Users/sifatul/coding/protean` host-machine dir `/home/hostmachine/data/protean/users/user-id/projects/protean`, mount them with each other `/home/hostmachine/data/protean/users/user-id/projects/protean:/Users/sifatul/coding/protean`.
  - Since we already mount our `/home/hostmachine/data/protean/users/user-id` to the container's `/workspace` the container already has access to everything that is being mounted within the directory.
  - If the user's chosen directory already exist in the remote host-machine then we will suffix the original directory name to be something unique. Then let the user know of the name we are using.

- The sandbox is always the one that executes the bash commands of the agent so that the user's machine is always safe.

- The container that sandboxes the agent will also allow HTTP and HTTPS traffic so that the agent can perform web scraping or searches or use tools that uses HTTP/WebSocket connections.

## The Agent

The agent needs to be highly testable in isolation with or without any database and tools. We should make use of the Vercel's AI SDK for this project of ours.

The system prompt of this agent will instruct it to be a explorer and problem solver than just a "helpful assistant". The agent's primary goal is to take in user requests look up for the available skills then start working on the task and keep working till it's done.

The agent also needs a way to store and preserve it's conversation history as well as auto compact the history when needed.

```ts
import { UIMessage } from "ai";

interface IThreadBase {
  id: string; // Auto generated. Uses ULID in a UUID compatible formatting.
  userId: string;
  title: string; // Auto generated after sending the first message in the thread.
  createdAt: string; // Auto generated use ISO time stamp
  updatedAt: string; // Auto generated on every update use ISO time stamp
  deletedAt: string; // Auto generated on delete use ISO time stamp
  // Note: the modelId depends on the inferenceProvider. The modelId and inferenceProvider changes together.
  modelId: string; // The ID of the model. E.g., openai/gpt-5.4, gpt-5.4, claude-opus-4.6, anthropic/claude-opus-4.6
  inferenceProvider: string; // The inference provider ID. E.g. openrouter, openai, anthropic.
}

interface IThreadUsage {
  id: string; // Auto generated. Uses ULID in UUID compatible format.
  threadId: string;
  createdAt: string; // Auto generated use ISO time stamp
  updatedAt: string; // Auto generated on every update use ISO time stamp
  deletedAt: string; // Auto generated on delete use ISO time stamp
  inputTokens: number;
  outputTokens: number;
  durationSeconds: number;
}

interface IThreadMessage {
  id: string; // ULID in UUID formatted string.
  threadId: string;
  createdAt: string; // Auto generated use ISO time stamp
  updatedAt: string; // Auto generated on every update use ISO time stamp
  deletedAt: string; // Auto generated on delete use ISO time stamp
  role: UIMessage["role"];
  parts: UIMessage["parts"];
  metadata: UIMessage["metadata"];
  // The thread message will also store the model selection information
  // so that if the same thread uses different models (changed by the user) we'd
  // know which model was uses for which message.
  modelId: string; // The ID of the model. E.g., openai/gpt-5.4, gpt-5.4, claude-opus-4.6, anthropic/claude-opus-4.6
  inferenceProvider: string; // The inference provider ID. E.g. openrouter, openai, anthropic.
}

interface IThreadSummary extends IThreadBase {
  usage: IThreadUsage;
}

interface IThread extends IThreadSummary {
  messages: IThreadMessage[];
}

interface IAgentMemory {
  getThreads: (userId: string) => Promise<IThreadSummary[]>
  getThread: (payload: { id: string; userId: string }) => Promise<IThreadSummary>
  getThreadWithMessages: (payload: { id: string; userId: string }) => Promise<IThread>
  createThread: (payload: Omit<IThreadBase, "id" | "createdAt" | "updatedAt"> & { userId: string }) => Promise<IThreadSummary>
  updateThread: (payload: Optional<IThreadBase> & { id: string; userId: string }) => Promise<IThreadSummary>
  updateThreadUsage: (
    payload: {
      userId: string;
      threadId: string;
      newInputTokens: number; // pass values like +1000 or -1000
      newOutputTokens: number;
      newDurationSeconds: number;
    },
  ) => Promise<IThreadUsage>
  deleteThread: (payload: { id: string; userId: string }) => Promise<void> // this only soft deletes
  upsertMessage: (
    payload: {
      userId: string;
      threadId: string;
      messageId?: string;
      role: UIMessage["role"];
      parts: UIMessage["parts"];
      metadata: UIMessage["metadata"];
    },
  ) => Promise<IThread>
  deleteMessage: (
    payload: {
      userId: string;
      threadId: string;
      messageId: string;
    }
  ) => Promise<IThread>
}

interface IAgentFactoryOption {
  memory: IAgentMemory
  logger: ILogger
}

interface IAgentFactory {
  (opts: IAgentFactoryOption) => Promise<IAgent>
}

interface IAgent {
  /**
   * The proper type of the async generator is defined by the AI-SDK and not by
   * us. This function will call the agent that is an instance of ToolLoopAgent
   * from AI-SDK and return the returning result. We may add some onError and
   * onFinish also maybe onStepFinish hook if available. But the caller of the
   * Agent.stream() should not be aware of this.
   */
  stream: () => Promise<ReturnTypeOf<typeof ToolLoopAgent["stream"]>>;
}
```

The agent memory can either be a SQLite database stored in the same workspace directory of the user in the host-machine. E.g. `/workspace/db.sqlite3` in reality this is a file stored in the `/home/hostmachine/data/protean/users/user-id/db.sqlite3`. Or could be a file system based memory where we save the data as JSON. The JSON data would go inside the same directory each thread will then be a JSON file. E.g. `/workspace/.threads/[id].json`. But I'll weigh on SQLite than JSON.

No matter what agent memory solution we choose we need to return the IAgentMemory interface from our IAgentMemoryFactory.

## Frontend of the system

The user facing frontend is a WebApp. For the web app we'll use Next.js with shadcn and AI-Elements. Use the frontend-skill along with vercel's ai-element skill and shadcn skill to prepare a top notch web app for our system.

As for the auth we'll use WorkOS to let the user's login with their google accounts only. Following is the wireframe of the webapp of ours.
(I'm also adding UI mockup images in ./ui-mockups for reference)

For the frontend we must use ai-elements and the built in components than write things ourselves.

The entire webapp is going to be a next.js app so we need to put our API logic in the API and the long running stuff + sandbox stuff in a different server APP that we'll host from our host-machine. We'll use fastify for any API work.

The entire project will be in Bun and should follow monorepo architecture. We should also apply the microservices philosophy.
