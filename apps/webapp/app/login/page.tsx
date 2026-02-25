import { signIn } from "@/auth";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

function GoogleLogo() {
  return (
    <svg
      aria-hidden="true"
      className="size-4"
      viewBox="0 0 48 48"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M47.53 24.55c0-1.64-.15-3.21-.43-4.73H24v8.95h13.2a11.28 11.28 0 0 1-4.9 7.4v6.15h7.93c4.65-4.28 7.3-10.6 7.3-17.77Z"
        fill="#4285F4"
      />
      <path
        d="M24 48c6.62 0 12.18-2.2 16.24-5.97l-7.93-6.15c-2.2 1.47-5.02 2.34-8.31 2.34-6.39 0-11.8-4.32-13.74-10.12H2.05v6.34A24 24 0 0 0 24 48Z"
        fill="#34A853"
      />
      <path
        d="M10.26 28.1A14.43 14.43 0 0 1 9.5 24c0-1.42.26-2.8.76-4.1v-6.34H2.05A24 24 0 0 0 0 24c0 3.86.93 7.5 2.05 10.44l8.21-6.34Z"
        fill="#FBBC05"
      />
      <path
        d="M24 9.54c3.6 0 6.83 1.24 9.37 3.67l7.02-7.02C36.18 2.2 30.62 0 24 0A24 24 0 0 0 2.05 13.56l8.21 6.34C12.2 13.86 17.61 9.54 24 9.54Z"
        fill="#EA4335"
      />
    </svg>
  );
}

export default function LoginPage() {
  return (
    <div className="flex min-h-screen bg-background">
      <main className="flex min-w-0 flex-1 items-center justify-center px-4">
        <Card className="w-full max-w-sm">
          <CardHeader className="text-center">
            <CardTitle className="text-2xl tracking-tight">
              Welcome back
            </CardTitle>
            <CardDescription>
              Sign in to continue to your account
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              action={async () => {
                "use server";
                await signIn(
                  "workos",
                  { redirectTo: "/dashboard" },
                  { provider: "GoogleOAuth" },
                );
              }}
            >
              <Button
                className="h-11 w-full justify-center"
                type="submit"
                variant="outline"
              >
                <GoogleLogo />
                Continue with Google
              </Button>
            </form>
          </CardContent>
        </Card>
      </main>
    </div>
  );
}
