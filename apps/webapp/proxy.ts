import { NextResponse, type NextFetchEvent, type NextRequest } from "next/server";

import { configureWorkOsEnv, isWorkOsEnabled } from "@/lib/server/workos";

const PUBLIC_PATHS = ["/login", "/login/start", "/callback", "/logout"];

export default async function proxy(request: NextRequest, event: NextFetchEvent) {
  configureWorkOsEnv();

  if (!isWorkOsEnabled()) {
    return NextResponse.next();
  }

  const { authkitMiddleware } = await import("@workos-inc/authkit-nextjs");
  const middleware = authkitMiddleware({
    redirectUri: process.env.NEXT_PUBLIC_WORKOS_REDIRECT_URI,
    middlewareAuth: {
      enabled: true,
      unauthenticatedPaths: PUBLIC_PATHS,
    },
  });

  return middleware(request, event);
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|ico)$).*)"],
};
