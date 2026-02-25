function toError(value: unknown): Error {
  if (value instanceof Error) {
    return value;
  }
  return new Error(String(value));
}

function createAbortError(reason?: unknown): Error {
  const message = reason
    ? `Operation aborted: ${String(reason)}`
    : "Operation aborted";
  const error = new Error(message);
  error.name = "AbortError";
  return error;
}

export async function runWithAbort<R>(
  operation: () => Promise<R> | R,
  signal?: AbortSignal,
): Promise<R> {
  if (!signal) {
    return await operation();
  }

  if (signal.aborted) {
    throw createAbortError(signal.reason);
  }

  return await new Promise<R>((resolve, reject) => {
    const onAbort = () => {
      reject(createAbortError(signal.reason));
    };

    signal.addEventListener("abort", onAbort, { once: true });

    Promise.resolve(operation())
      .then(resolve)
      .catch((error: unknown) => {
        reject(toError(error));
      })
      .finally(() => {
        signal.removeEventListener("abort", onAbort);
      });
  });
}

export async function tryCatch<R>(
  operation: () => Promise<R> | R,
): Promise<{ result: R; error: Error | null }> {
  try {
    const result = await operation();
    return {
      result,
      error: null,
    };
  } catch (error: unknown) {
    return {
      result: null as unknown as R,
      error: toError(error),
    };
  }
}
