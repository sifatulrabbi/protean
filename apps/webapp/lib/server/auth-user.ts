import { auth } from "@/auth";

export async function requireUserId() {
  const session = await auth();

  if (!session?.user?.email) {
    return null;
  }

  return session.user.email;
}
