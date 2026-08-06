function sanitizeSecret(value?: string) {
  return value?.replace(/^"+|"+$/g, "");
}

export function configureWorkOsEnv() {
  process.env.WORKOS_COOKIE_PASSWORD ||= sanitizeSecret(process.env.AUTH_SECRET) || "protean-local-cookie-password-32-bytes";
  process.env.NEXT_PUBLIC_WORKOS_REDIRECT_URI ||= process.env.WORKOS_REDIRECT_URI || "http://localhost:3000/callback";
}

export function isWorkOsEnabled() {
  return Boolean(process.env.WORKOS_API_KEY && process.env.WORKOS_CLIENT_ID);
}

export function getApiBaseUrl() {
  return process.env.API_BASE_URL || "http://127.0.0.1:8788";
}

export async function getServerAuth() {
  configureWorkOsEnv();

  if (!isWorkOsEnabled()) {
    return { user: null };
  }

  try {
    const { withAuth } = await import("@workos-inc/authkit-nextjs");
    return await withAuth();
  } catch {
    return { user: null };
  }
}
