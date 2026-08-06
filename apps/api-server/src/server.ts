import Fastify from "fastify";
import type { FastifyReply, FastifyRequest } from "fastify";
import cors from "@fastify/cors";
import { createFileAgentMemory } from "@protean/agent-memory";
import { readAvailableSkills, runAgentTurn } from "@protean/bash-agent";
import {
  chatRequestSchema,
  threadCreateSchema,
  threadSchema,
  threadSummarySchema,
} from "@protean/contracts";
import { createProject, createSandboxClient } from "@protean/sandbox-sdk";
import { z } from "zod";

import { getApiServerConfig } from "./config";

const userQuerySchema = z.object({
  userId: z.string().min(1),
});

const threadParamsSchema = z.object({
  threadId: z.string().min(1),
});

const remoteProjectCreateSchema = z.object({
  projectName: z.string().min(1),
  seedReadme: z.boolean().default(true),
});

const mountProjectSchema = z.object({
  sourcePath: z.string().min(1),
  projectName: z.string().min(1),
});

const projectTreeParamsSchema = z.object({
  projectName: z.string().min(1),
});

const projectTreeQuerySchema = z.object({
  path: z.string().optional(),
});

const projectFileQuerySchema = z.object({
  path: z.string().min(1),
});

const routeThreadCreateSchema = threadCreateSchema.extend({
  title: z.string().min(1).optional(),
});

function sanitizeProjectName(name: string) {
  const slug = name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "");

  if (!slug) {
    throw new Error("Project name must contain letters or numbers.");
  }

  return slug;
}

function createRemoteProjectReadme(projectName: string) {
  return `# ${projectName}

This project was created in Protean.

- Storage mode: Hosted remote workspace
- Sandbox path: /workspace/projects/${projectName}
`;
}

export async function createApiServer() {
  const config = getApiServerConfig();
  const sandboxClient = createSandboxClient({
    baseUrl: config.sandboxBaseUrl,
    token: config.sandboxToken,
  });
  const memory = await createFileAgentMemory({ baseDir: config.dataDir });

  const app = Fastify({
    logger: true,
  });

  await app.register(cors, {
    origin: config.webappOrigin,
  });

  app.get("/health", async () => {
    try {
      const sandbox = await sandboxClient.getHealth();

      return {
        ok: true,
        apiBaseUrl: config.apiBaseUrl,
        sandbox,
      };
    } catch (error) {
      return {
        ok: false,
        apiBaseUrl: config.apiBaseUrl,
        sandbox: null,
        error:
          error instanceof Error ? error.message : "Sandbox is unavailable.",
      };
    }
  });

  app.get("/skills", async () => readAvailableSkills(config.skillsFilePath));

  app.get("/threads", async (request: FastifyRequest) => {
    const query = userQuerySchema.parse(request.query);

    return z
      .array(threadSummarySchema)
      .parse(await memory.getThreads(query.userId));
  });

  app.get("/threads/:threadId", async (request: FastifyRequest) => {
    const params = threadParamsSchema.parse(request.params);
    const query = userQuerySchema.parse(request.query);

    return threadSchema.parse(
      await memory.getThreadWithMessages({
        id: params.threadId,
        userId: query.userId,
      }),
    );
  });

  app.post("/threads", async (request: FastifyRequest) => {
    const body = routeThreadCreateSchema.parse(request.body);

    return threadSummarySchema.parse(
      await memory.createThread({
        userId: body.userId,
        title: body.title || "New thread",
        deletedAt: null,
        modelId: body.modelId,
        inferenceProvider: body.inferenceProvider,
      }),
    );
  });

  app.delete(
    "/threads/:threadId",
    async (request: FastifyRequest, reply: FastifyReply) => {
      const params = threadParamsSchema.parse(request.params);
      const query = userQuerySchema.parse(request.query);

      await memory.deleteThread({ id: params.threadId, userId: query.userId });

      reply.code(204);
      return null;
    },
  );

  app.get("/projects", async () => sandboxClient.listProjects());

  app.post("/projects/remote", async (request: FastifyRequest) => {
    const body = remoteProjectCreateSchema.parse(request.body);
    const projectName = sanitizeProjectName(body.projectName);
    const fullPath = `/workspace/projects/${projectName}`;
    const existing = await sandboxClient.fs.stat(fullPath);

    if (existing.entity) {
      throw new Error(`Project "${projectName}" already exists.`);
    }

    await sandboxClient.fs.create(fullPath, "directory", { recursive: true });

    if (body.seedReadme) {
      await sandboxClient.fs.writeFile(
        `${fullPath}/README.md`,
        createRemoteProjectReadme(projectName),
      );
    }

    return {
      name: projectName,
      fullPath,
      source: "remote" as const,
      mountId: null,
    };
  });

  app.post("/projects/mount", async (request: FastifyRequest) => {
    const body = mountProjectSchema.parse(request.body);

    return sandboxClient.createMount({
      sourcePath: body.sourcePath,
      projectName: sanitizeProjectName(body.projectName),
    });
  });

  app.get("/projects/:projectName/tree", async (request: FastifyRequest) => {
    const params = projectTreeParamsSchema.parse(request.params);
    const query = projectTreeQuerySchema.parse(request.query);
    const project = createProject({
      sandboxClient,
      projectPath: `/workspace/projects/${params.projectName}`,
    });

    return project.fs.listDir(query.path || ".");
  });

  app.get("/projects/:projectName/file", async (request: FastifyRequest) => {
    const params = projectTreeParamsSchema.parse(request.params);
    const query = projectFileQuerySchema.parse(request.query);
    const project = createProject({
      sandboxClient,
      projectPath: `/workspace/projects/${params.projectName}`,
    });

    return project.fs.readFile(query.path, { startLine: 0, endLine: 200 });
  });

  app.post("/chat", async (request: FastifyRequest) => {
    const body = chatRequestSchema.parse(request.body);

    return runAgentTurn({
      ...body,
      memory,
      sandboxClient,
      skillsFilePath: config.skillsFilePath,
    });
  });

  return { app, config };
}

if (import.meta.main) {
  const { app, config } = await createApiServer();

  await app.listen({
    host: config.host,
    port: config.port,
  });
}

export { sanitizeProjectName };
