import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export function proxy(request: NextRequest) {
  // Authentication redirects live in the client guard. API credentials are header
  // only and no longer mirrored into a JavaScript-readable routing cookie.
  void request;
  return NextResponse.next();
}

export const config = {
  // Skip Next.js internals, API routes, the favicon, and static files under public/ (including images and fonts).
  matcher: ["/((?!_next/static|_next/image|favicon\\.ico|api/|.*\\.(?:png|jpg|jpeg|gif|webp|svg|ico|woff2?|ttf|otf)$).*)"],
};
