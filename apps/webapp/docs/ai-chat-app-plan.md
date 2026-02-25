# AI Chat App: Robust + Scalable Build Plan

## 1) Goals and Non-Goals

### Goals

- Build a production-grade multi-user AI chat SaaS on Next.js.
- Use Vercel AI SDK for streaming/chat primitives.
- Use OpenRouter for LLM inference with model routing and fallback.
- Ship with strong auth, persistence, observability, and cost controls.
- Design for gradual scale-up without major rewrites.

### Non-Goals (initial phases)

- Agentic workflow orchestration with long-running background jobs.
- Enterprise RBAC/SCIM/SAML admin tooling.
- Complex multi-tenant billing customization.

## 2) Product Scope (v1)

- Authentication and session management (WorkOS + NextAuth).
- Chat threads with persisted message history.
- Streaming assistant responses (token-by-token).
- Model selector (safe curated list from OpenRouter).
- Conversation controls: retry, stop generation, edit and resend user prompt.
- Basic usage metering (requests, tokens, estimated cost).

## 3) Architecture Overview

### Runtime

- Frontend/API: Next.js App Router on Vercel.
- SDK: Vercel AI SDK for chat transport + stream handling.
- Inference: OpenRouter Chat Completions-compatible endpoint.
- Auth: NextAuth + WorkOS.
- DB: Drizzle ORM; start with SQLite for local/dev, plan migration to Postgres for production scale.

### Core Components

- `ChatFacade` pattern: single orchestration layer for chat requests.
- `ProviderFactory` pattern: constructs model clients (OpenRouter now, extensible later).
- `SafetyPipeline` pattern: prompt input checks + output checks.
- `UsageMeter` singleton service: token/cost accounting.
- `PersistenceRepository` pattern: chat/thread/message storage isolation.

## 4) Data Model (Drizzle)

### Core Tables

- `users`: id, email, name, createdAt.
- `accounts/sessions` (NextAuth tables as needed).
- `chat_threads`: id, userId, title, createdAt, updatedAt, archivedAt.
- `chat_messages`: id, threadId, role (`user|assistant|system|tool`), content, model, tokenCounts, latencyMs, createdAt.
- `chat_generations`: id, threadId, request metadata (model, temperature, maxTokens), status, errorCode, createdAt.
- `usage_events`: id, userId, threadId, provider, model, inputTokens, outputTokens, estimatedCostUsd, createdAt.

### Indexing

- `chat_threads(userId, updatedAt DESC)`.
- `chat_messages(threadId, createdAt ASC)`.
- `usage_events(userId, createdAt DESC)`.

## 5) API + Streaming Design

### Primary Endpoint

- `POST /api/chat`
  - Auth required.
  - Input: `threadId`, `messages`, `modelId`, generation params.
  - Flow:
    1. Validate input schema (`zod`).
    2. Authorize ownership (`thread.userId === session.user.id`).
    3. Persist incoming user message.
    4. Call `streamText` (Vercel AI SDK) with OpenRouter model.
    5. Stream partial tokens to client.
    6. Persist final assistant message + usage metrics.

### Supporting Endpoints

- `GET /api/threads` list user threads.
- `POST /api/threads` create thread.
- `GET /api/threads/:id/messages`.
- `DELETE /api/threads/:id` soft delete/archive.

## 6) Reliability and Failure Strategy

- Timeouts on provider calls with clear retryable errors.
- Model fallback chain (e.g., primary -> backup) for provider/model outages.
- Idempotency key on message submit to prevent duplicate assistant generations.
- Graceful stream termination handling (`AbortController`).
- Structured error taxonomy: `AUTH`, `VALIDATION`, `PROVIDER`, `RATE_LIMIT`, `UNKNOWN`.

## 7) Security and Privacy

- Enforce auth on all chat and thread routes.
- Validate thread ownership server-side on every request.
- Keep secrets server-only (`OPENROUTER_API_KEY`, auth secrets).
- Redact PII in logs.
- Add configurable retention policy for chat history.

## 8) Performance and Scalability Plan

### Phase A (Current scale: low traffic)

- SQLite acceptable for local and very small environments.
- Basic caching for model metadata.
- Stream responses directly from server route.

### Phase B (Growing usage)

- Migrate to Postgres (Neon/Supabase/managed DB) with Drizzle migrations.
- Add Redis for:
  - request-level rate limiting,
  - short-term idempotency keys,
  - hot thread metadata caching.
- Add background queue for non-critical writes/analytics.

### Phase C (High scale)

- Partition usage events by date.
- Move heavy analytics to warehouse pipeline.
- Introduce circuit breakers + provider health probes.

## 9) Observability and Ops

- Structured logging with request ID, user ID, thread ID.
- Metrics:
  - p50/p95 latency per model.
  - error rate by error code.
  - token usage and cost/day.
  - stream cancel rate.
- Alerting:
  - spike in `PROVIDER` errors,
  - unusual cost growth,
  - auth failure spikes.

## 10) Cost Controls

- Per-user daily/monthly token budgets.
- Model allowlist by plan tier.
- Max output tokens cap.
- Soft and hard rate limits.
- Dashboard view for usage transparency.

## 11) UX and Frontend Plan

- Chat layout with virtualized message list when threads get large.
- Optimistic UI for user messages; streamed assistant response bubble.
- Regenerate and edit-last-message flows.
- Persist draft input per thread.
- Accessibility pass: keyboard navigation, screen-reader labels, contrast.

## 12) Testing Strategy

### Unit

- Prompt assembly, validation, usage metering, fallback logic.

### Integration

- `/api/chat` authenticated success flow.
- Provider failure and fallback behavior.
- Thread authorization checks.

### E2E

- Sign in -> create thread -> send message -> stream response -> reload history.
- Multi-tab behavior and race conditions.

## 13) Delivery Phases (Execution Roadmap)

### Phase 1: Foundation (Week 1)

- Set up DB schema + migrations (threads/messages/usage).
- Implement auth-guarded thread APIs.
- Build basic chat UI skeleton.
- Done criteria: user can create thread and view persisted empty thread.

### Phase 2: Streaming Chat Core (Week 2)

- Implement `/api/chat` with Vercel AI SDK + OpenRouter.
- Persist user and assistant messages.
- Add stop/retry/edit-last flows.
- Done criteria: stable streaming chat with persisted history.

### Phase 3: Reliability + Guardrails (Week 3)

- Fallback models, timeout handling, idempotency keys.
- Add rate limits and token budgets.
- Done criteria: graceful degradation under provider failures.

### Phase 4: Observability + Cost + Hardening (Week 4)

- Metrics, logs, error tracking, usage dashboard basics.
- Security review and retention controls.
- Done criteria: production readiness checklist passes.

## 14) Immediate Next Implementation Tasks

1. Add `vercel/ai` and OpenRouter provider integration package(s).
2. Define Drizzle schema + initial migration.
3. Build `ChatFacade` and `ProviderFactory` abstractions.
4. Implement `POST /api/chat` streaming route with persistence hooks.
5. Build thread list + chat pane UI with real backend wiring.
6. Add usage metering writes and a simple per-user usage page.

## 15) Risks and Mitigations

- Risk: SQLite bottlenecks in production.
  - Mitigation: plan Postgres migration by Phase 3, keep repository abstraction clean.
- Risk: rising inference costs.
  - Mitigation: token caps, model allowlists, budget enforcement.
- Risk: provider instability.
  - Mitigation: fallback chain + circuit breaker + alerting.

---

This plan is optimized for shipping quickly while preserving a clean path to scale without architecture rewrites.
