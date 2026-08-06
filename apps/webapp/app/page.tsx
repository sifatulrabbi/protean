import { redirect } from "next/navigation";

import { DemoBanner } from "@/components/product/demo-banner";
import { WorkspaceShell } from "@/components/product/workspace-shell";
import { getApiBaseUrl, getServerAuth, isWorkOsEnabled } from "@/lib/server/workos";

export const dynamic = "force-dynamic";

export default async function Home() {
  const authEnabled = isWorkOsEnabled();
  const auth = await getServerAuth();
  const user = auth.user;

  if (authEnabled && !user) redirect("/login");

  return (
    <>
      {!authEnabled && <DemoBanner />}
      <WorkspaceShell
        apiBaseUrl={getApiBaseUrl()}
        viewer={{
          authMode: user ? "workos" : "demo",
          avatarUrl: user?.profilePictureUrl ?? null,
          email: user?.email ?? "local@protean.dev",
          id: user?.id ?? "local-demo-user",
          name: user?.firstName || user?.email || "Local operator",
        }}
      />
    </>
  );
}
