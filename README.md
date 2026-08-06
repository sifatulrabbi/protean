# Protean

A sandboxed, per-project AI agent workspace. Users create projects inside organizations; each project gets its own filesystem and its own sandbox where the agent executes. Users and the agent work on the same files.

## Layout

- `backend/` — Go API, agent harness, and sandbox orchestration (hexagonal: `internal/ports` + `internal/adapters`, wired at boot).
- `frontend/` — React app (Vite + TypeScript).
- `sandbox-image/` — base OCI image the per-project sandboxes run (bash, git, curl, ripgrep, fd; non-root `agent` user).

## Development

Copy `.env.example` to `.env` and fill in the keys, then:

- `make dev` — run the API (port 8788) and the frontend (port 3000) together.
- `make test` / `make lint` / `make build` — what CI runs.
- `make sandbox-image` — build the sandbox base image (needs a Docker daemon).

Requires Go 1.26+, Node 22+, pnpm 10+.
