import { auth } from "@/auth";
import { SettingsShell } from "@/components/settings/settings-shell";
import { redirect } from "next/navigation";

export default async function SettingsPage() {
  const session = await auth();

  if (!session?.user) {
    redirect("/login");
  }

  return <SettingsShell />;
}
