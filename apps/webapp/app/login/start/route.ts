import { redirect } from "next/navigation";

import { configureWorkOsEnv, isWorkOsEnabled } from "@/lib/server/workos";

export async function GET() {
  configureWorkOsEnv();

  if (!isWorkOsEnabled()) {
    redirect("/");
  }

  const { getSignInUrl } = await import("@workos-inc/authkit-nextjs");

  redirect(await getSignInUrl({ returnTo: "/" }));
}
