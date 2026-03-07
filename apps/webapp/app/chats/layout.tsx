import { auth } from "@/auth";
import { ThreadsSidebar } from "@/components/chat/threads-sidebar";
import { getAgentMemory } from "@/lib/server/agent-memory";
import { getWorkspaceSandbox } from "@/lib/server/workspace-fs";
import { redirect } from "next/navigation";

export default async function ChatsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const session = await auth();

  if (!session?.user) {
    redirect("/login");
  }

  const userEmail = session.user.email;

  if (!userEmail) {
    redirect("/login");
  }

  await getWorkspaceSandbox(userEmail);
  const memory = await getAgentMemory(userEmail);
  const threads = await memory.listThreads({ userId: userEmail });

  return (
    <div className="flex min-h-screen">
      <ThreadsSidebar threads={threads} />

      <main className="min-w-0 flex-1">
        <div className="flex h-screen min-h-0 flex-col bg-background">
          <div className="flex min-h-0 w-full flex-1 px-3 pt-14 md:px-4 md:pt-4">
            {children}
          </div>
        </div>
      </main>
    </div>
  );
}
