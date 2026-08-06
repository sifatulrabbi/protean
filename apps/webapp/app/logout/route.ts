import { redirect } from "next/navigation";

import { configureWorkOsEnv, isWorkOsEnabled } from "@/lib/server/workos";

export async function GET() {
  configureWorkOsEnv();

  if (!isWorkOsEnabled()) {
    redirect("/");
  }

  const { signOut } = await import("@workos-inc/authkit-nextjs");

  await signOut({ returnTo: "/" });
}
