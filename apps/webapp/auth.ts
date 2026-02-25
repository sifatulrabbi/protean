import NextAuth from "next-auth";
import WorkOS from "next-auth/providers/workos";

export const { handlers, auth, signIn, signOut } = NextAuth({
  trustHost: true,
  providers: [
    WorkOS({
      clientId: process.env.WORKOS_CLIENT_ID,
      clientSecret:
        process.env.WORKOS_CLIENT_SECRET ?? process.env.WORKOS_API_KEY,
    }),
  ],
  pages: {
    signIn: "/login",
  },
  callbacks: {
    authorized({ auth }) {
      const allowedUsersStr =
        process.env.ALLOWED_ALPHA_USERS || "sifatuli.r@gmail.com";
      const allowedUsers = allowedUsersStr.replaceAll(", ", ",").split(",");
      return !!auth?.user?.email && allowedUsers.includes(auth.user.email);
    },
  },
});
