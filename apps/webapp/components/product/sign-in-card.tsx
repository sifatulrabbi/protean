import { ArrowRightIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function SignInCard() {
  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm border-border/60 bg-card/80 shadow-[0_30px_120px_-40px_rgba(15,23,42,0.35)] backdrop-blur">
        <CardHeader className="items-center text-center">
          <Badge className="mb-2 rounded-full bg-primary/12 px-3 py-1 text-primary hover:bg-primary/12" variant="secondary">
            Protean
          </Badge>
          <CardTitle className="text-xl">Sign in to your workspace</CardTitle>
          <CardDescription>Authenticate with your Google account to access the operator workspace.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <a
            className="inline-flex w-full items-center justify-center gap-2 rounded-full bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90"
            href="/login/start"
          >
            Continue with Google
            <ArrowRightIcon className="size-4" />
          </a>
          <p className="text-center text-xs text-muted-foreground">Secured by WorkOS</p>
        </CardContent>
      </Card>
    </main>
  );
}
