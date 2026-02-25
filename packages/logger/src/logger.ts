export interface Logger {
  info(message: string, meta?: unknown): void;
  warn(message: string, meta?: unknown): void;
  error(message: string, meta?: unknown): void;
  debug(message: string, meta?: unknown): void;
}

type LogLevel = "info" | "warn" | "error" | "debug";

function formatMeta(meta: unknown): unknown {
  if (meta instanceof Error) {
    return {
      name: meta.name,
      message: meta.message,
      stack: meta.stack,
    };
  }

  return meta;
}

function buildPrefix(level: LogLevel): string {
  return `[${new Date().toISOString()}] [${level.toUpperCase()}]`;
}

function writeConsole(method: LogLevel, message: string, meta?: unknown): void {
  if (method === "debug" && process.env.NODE_ENV === "production") {
    return;
  }

  const prefix = buildPrefix(method);
  const output = `${prefix} ${message}`;

  if (typeof meta === "undefined") {
    console[method](output);
    return;
  }

  console[method](output, formatMeta(meta));
}

export const noopLogger: Logger = {
  info: () => {},
  warn: () => {},
  error: () => {},
  debug: () => {},
};

export const consoleLogger: Logger = {
  info: (message, meta) => writeConsole("info", message, meta),
  warn: (message, meta) => writeConsole("warn", message, meta),
  error: (message, meta) => writeConsole("error", message, meta),
  debug: (message, meta) => writeConsole("debug", message, meta),
};
