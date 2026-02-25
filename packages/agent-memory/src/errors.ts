/**
 * Typed error thrown by all agent-memory operations.
 *
 * Using a discriminated `code` field instead of separate subclasses keeps
 * error handling simple: callers can switch/match on `code` without
 * needing to import multiple error types.
 */
export class ThreadMemoryError extends Error {
  readonly code:
    | "THREAD_NOT_FOUND" // The requested thread does not exist on disk
    | "INVALID_STATE" // e.g. duplicate thread id, malformed thread id
    | "READ_ERROR" // Unexpected I/O failure while reading a thread file
    | "WRITE_ERROR" // Unexpected I/O failure while writing a thread file
    | "VALIDATION_ERROR"; // Persisted JSON failed Zod schema validation
  readonly cause?: unknown;

  constructor(
    code: ThreadMemoryError["code"],
    message: string,
    cause?: unknown,
  ) {
    super(message);
    this.name = "ThreadMemoryError";
    this.code = code;
    this.cause = cause;
  }
}
