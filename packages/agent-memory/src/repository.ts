import type { AgentMemory } from "./types";

/** Re-exported here for consumers who import from this module path. */
export type { AgentMemory as FsMemory };

/**
 * Marker interface for versioned persisted state objects.
 * Extend this when adding new top-level persistence structures that
 * need a format version for future migrations.
 */
export interface PersistedThreadState {
  version: 1;
}
