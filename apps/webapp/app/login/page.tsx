import { redirect } from "next/navigation";

import { SignInCard } from "@/components/product/sign-in-card";
import { getServerAuth, isWorkOsEnabled } from "@/lib/server/workos";

export default async function LoginPage() {
  if (!isWorkOsEnabled()) redirect("/");

  const auth = await getServerAuth();
  if (auth.user) redirect("/");

  return <SignInCard />;
}
