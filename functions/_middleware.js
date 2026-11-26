// Cloudflare Pages Middleware: injects data-theme="light|dark" on <html> when
// the user selected an explicit theme via /theme (zero client-side JavaScript).

function readThemeCookie(cookieHeader) {
  if (!cookieHeader) return "";
  for (const part of cookieHeader.split(";")) {
    const idx = part.indexOf("=");
    if (idx === -1) continue;
    const key = part.slice(0, idx).trim();
    if (key === "kryx_theme") {
      const val = part.slice(idx + 1).trim().toLowerCase();
      if (val === "light" || val === "dark") return val;
      return "";
    }
  }
  return "";
}

export async function onRequest(context) {
  const response = await context.next();
  const theme = readThemeCookie(context.request.headers.get("cookie"));
  if (!theme) return response;

  const ct = (response.headers.get("content-type") || "").toLowerCase();
  if (!ct.includes("text/html")) return response;

  const text = await response.text();
  const updated = text.replace(
    '<html lang="en">',
    `<html lang="en" data-theme="${theme}">`
  );

  const headers = new Headers(response.headers);
  headers.delete("content-length");
  const prevVary = headers.get("vary");
  if (!prevVary) {
    headers.set("vary", "Cookie");
  } else if (!prevVary.toLowerCase().includes("cookie")) {
    headers.set("vary", `${prevVary}, Cookie`);
  }

  return new Response(updated, {
    status: response.status,
    statusText: response.statusText,
    headers,
  });
}
