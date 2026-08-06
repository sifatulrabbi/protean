import { describe, expect, it } from "bun:test";

import { sanitizeProjectName } from "../src/server";

describe("sanitizeProjectName", () => {
  it("normalizes project names into safe slugs", () => {
    expect(sanitizeProjectName("Protean Docs")).toBe("protean-docs");
  });

  it("rejects empty slugs", () => {
    expect(() => sanitizeProjectName("!!!")).toThrow("Project name must contain letters or numbers.");
  });
});
