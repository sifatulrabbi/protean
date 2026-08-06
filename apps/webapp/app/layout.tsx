import type { Metadata } from "next";
import { AuthKitProvider } from "@workos-inc/authkit-nextjs/components";
import { IBM_Plex_Mono, Space_Grotesk } from "next/font/google";

import { TooltipProvider } from "@/components/ui/tooltip";
import { getServerAuth } from "@/lib/server/workos";
import "./globals.css";

const spaceGrotesk = Space_Grotesk({
  variable: "--font-space-grotesk",
  subsets: ["latin"],
});

const plexMono = IBM_Plex_Mono({
  variable: "--font-plex-mono",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

export const metadata: Metadata = {
  title: "Protean",
  description: "An AI workspace for hosted and self-hosted project execution.",
};

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const initialAuth = await getServerAuth();

  return (
    <html lang="en">
      <body className={`${spaceGrotesk.variable} ${plexMono.variable} antialiased`}>
        <AuthKitProvider initialAuth={initialAuth}>
          <TooltipProvider>{children}</TooltipProvider>
        </AuthKitProvider>
      </body>
    </html>
  );
}
