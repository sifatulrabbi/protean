import type { NextRequest } from "next/server";

import { configureWorkOsEnv } from "@/lib/server/workos";

export async function GET(request: NextRequest) {
  configureWorkOsEnv();

  const { handleAuth } = await import("@workos-inc/authkit-nextjs");

  return handleAuth({ returnPathname: "/" })(request);
}
