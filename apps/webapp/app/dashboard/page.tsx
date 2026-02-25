import { auth, signOut } from "@/auth";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { DataToolsSection } from "@/components/dashboard/data-tools-section";
import { UsageSection } from "@/components/dashboard/usage-section";
import { ArrowLeftIcon, LogOutIcon, UserIcon } from "lucide-react";
import Link from "next/link";
import { redirect } from "next/navigation";

export default async function DashboardPage() {
  const session = await auth();

  if (!session?.user) {
    redirect("/login");
  }

  const userEmail = session.user.email ?? "Unknown user";
  const userName = session.user.name?.trim() || "User";
  const userInitials = userName
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");

  return (
    <div className="flex min-h-screen bg-background">
      <main className="mx-auto flex min-h-screen w-full max-w-4xl flex-1 flex-col px-3 pt-14 pb-4 md:px-4 md:pt-6">
        <div className="mb-4 flex items-center justify-between gap-2">
          <h1 className="font-semibold text-xl tracking-tight">Dashboard</h1>
          <Button asChild size="sm" variant="outline">
            <Link href="/chats/new">
              <ArrowLeftIcon className="size-4" />
              Back to chat
            </Link>
          </Button>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <UserIcon className="size-4" />
                User info
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center gap-3">
                <Avatar size="lg">
                  <AvatarImage
                    alt={userName}
                    src={session.user.image ?? undefined}
                  />
                  <AvatarFallback>{userInitials || "U"}</AvatarFallback>
                </Avatar>
                <div className="min-w-0">
                  <p className="truncate font-medium text-sm">{userName}</p>
                  <p className="truncate text-muted-foreground text-sm">
                    {userEmail}
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>

          <DataToolsSection />

          <UsageSection />
        </div>

        <div className="mt-auto pt-6">
          <Separator />
          <form
            action={async () => {
              "use server";
              await signOut({ redirectTo: "/login" });
            }}
            className="pt-4"
          >
            <Button className="w-full justify-center sm:w-auto" type="submit">
              <LogOutIcon className="size-4" />
              Log out
            </Button>
          </form>
        </div>
      </main>
    </div>
  );
}
