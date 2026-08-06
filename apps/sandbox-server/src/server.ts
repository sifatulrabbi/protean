import Fastify from "fastify";
import {
  sandboxCopyRequestSchema,
  sandboxCreateRequestSchema,
  sandboxExecRequestSchema,
  sandboxHealthSchema,
  sandboxMoveRequestSchema,
  sandboxReadFileRequestSchema,
  sandboxRemoveRequestSchema,
  sandboxWriteFileRequestSchema,
} from "@protean/contracts";
import { z } from "zod";

import { getSandboxServerConfig } from "./config";
import {
  copyVirtualPath,
  createMountedProject,
  createVirtualEntity,
  ensureWorkspace,
  executeScopedCommand,
  listProjects,
  listVirtualDir,
  readVirtualFile,
  removeVirtualPath,
  statVirtualPath,
  writeVirtualFile,
  moveVirtualPath,
} from "./filesystem";

const statRequestSchema = z.object({
  fullPath: z.string().min(1),
});

const listDirRequestSchema = z.object({
  fullPath: z.string().min(1),
});

const mountRequestSchema = z.object({
  sourcePath: z.string().min(1),
  projectName: z.string().min(1),
});

export async function createSandboxServer() {
  const config = getSandboxServerConfig();

  await ensureWorkspace(config);

  const app = Fastify({
    logger: true,
  });

  app.addHook("onRequest", async (request, reply) => {
    if (
      config.serviceToken &&
      request.headers.authorization !== `Bearer ${config.serviceToken}`
    ) {
      return reply.code(401).send({ error: "Unauthorized" });
    }
  });

  app.get("/health", async () =>
    sandboxHealthSchema.parse({
      ok: true,
      workspaceRoot: config.virtualWorkspaceRoot,
      projectsRoot: `${config.virtualWorkspaceRoot}/projects`,
      mountsRoot: `${config.virtualWorkspaceRoot}/mounts`,
    }),
  );

  app.get("/projects", async () => listProjects(config));

  app.post("/mounts", async (request) => {
    const body = mountRequestSchema.parse(request.body);

    return createMountedProject(config, body);
  });

  app.post("/bash/exec", async (request) => {
    const body = sandboxExecRequestSchema.parse(request.body);

    return executeScopedCommand(config, body);
  });

  app.post("/fs/read-file", async (request) => {
    const body = sandboxReadFileRequestSchema.parse(request.body);

    try {
      return {
        ...(await readVirtualFile(config, body.fullPath, body.range)),
        error: null,
      };
    } catch (error) {
      return {
        fullPath: body.fullPath,
        content: "",
        range: body.range,
        error: error instanceof Error ? error.message : "Failed to read file.",
      };
    }
  });

  app.post("/fs/list-dir", async (request) => {
    const body = listDirRequestSchema.parse(request.body);

    try {
      return {
        ...(await listVirtualDir(config, body.fullPath)),
        error: null,
      };
    } catch (error) {
      return {
        fullPath: body.fullPath,
        entries: [],
        error:
          error instanceof Error ? error.message : "Failed to list directory.",
      };
    }
  });

  app.post("/fs/stat", async (request) => {
    const body = statRequestSchema.parse(request.body);

    try {
      return {
        ...(await statVirtualPath(config, body.fullPath)),
        error: null,
      };
    } catch (error) {
      return {
        fullPath: body.fullPath,
        entity: null,
        error: error instanceof Error ? error.message : "Failed to stat path.",
      };
    }
  });

  app.post("/fs/write-file", async (request) => {
    const body = sandboxWriteFileRequestSchema.parse(request.body);

    try {
      return {
        ...(await writeVirtualFile(config, body.fullPath, body.content)),
        error: null,
      };
    } catch (error) {
      return {
        fullPath: body.fullPath,
        error: error instanceof Error ? error.message : "Failed to write file.",
      };
    }
  });

  app.post("/fs/create", async (request) => {
    const body = sandboxCreateRequestSchema.parse(request.body);

    try {
      return {
        ...(await createVirtualEntity(
          config,
          body.fullPath,
          body.entityType,
          body.opts?.recursive,
        )),
        error: null,
      };
    } catch (error) {
      return {
        fullPath: body.fullPath,
        entity: null,
        error:
          error instanceof Error ? error.message : "Failed to create entity.",
      };
    }
  });

  app.post("/fs/copy", async (request) => {
    const body = sandboxCopyRequestSchema.parse(request.body);

    try {
      return {
        ...(await copyVirtualPath(
          config,
          body.sourceFullPath,
          body.copyToFullPath,
        )),
        error: null,
      };
    } catch (error) {
      return {
        sourceFullPath: body.sourceFullPath,
        copyToFullPath: body.copyToFullPath,
        error: error instanceof Error ? error.message : "Failed to copy path.",
      };
    }
  });

  app.post("/fs/move", async (request) => {
    const body = sandboxMoveRequestSchema.parse(request.body);

    try {
      return {
        ...(await moveVirtualPath(
          config,
          body.sourceFullPath,
          body.newFullPath,
        )),
        error: null,
      };
    } catch (error) {
      return {
        sourceFullPath: body.sourceFullPath,
        newFullPath: body.newFullPath,
        error: error instanceof Error ? error.message : "Failed to move path.",
      };
    }
  });

  app.post("/fs/remove", async (request) => {
    const body = sandboxRemoveRequestSchema.parse(request.body);

    try {
      return {
        ...(await removeVirtualPath(
          config,
          body.fullPath,
          body.opts?.recursive,
        )),
        error: null,
      };
    } catch (error) {
      return {
        fullPath: body.fullPath,
        error:
          error instanceof Error ? error.message : "Failed to remove path.",
      };
    }
  });

  return { app, config };
}

if (import.meta.main) {
  const { app, config } = await createSandboxServer();

  await app.listen({
    host: config.host,
    port: config.port,
  });
}
