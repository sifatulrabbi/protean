export { auth as proxy } from "@/auth";

export const config = {
  matcher: ["/((?!api|agent|_next/static|_next/image|favicon.ico|login).*)"],
};
