// Cloudflare Pages Function: zero-JS edge theme switcher at /theme.
// Sets or clears the kryx_theme cookie and redirects safely to a relative path.

import { safeNextPath } from "./_shared.js";

export async function onRequestGet(context) {
  const reqURL = new URL(context.request.url);
  const mode = (reqURL.searchParams.get("mode") || "").trim().toLowerCase();
  const next = safeNextPath(reqURL.searchParams.get("next"));

  const cookie =
    mode === "light" || mode === "dark"
      ? `kryx_theme=${mode}; Path=/; Max-Age=31536000; SameSite=Lax`
      : "kryx_theme=; Path=/; Max-Age=0; SameSite=Lax";

  return new Response(null, {
    status: 302,
    headers: {
      location: next,
      "set-cookie": cookie,
      "cache-control": "no-store",
    },
  });
}
