"use client";

import * as React from "react";

import Link from "next/link";

import { ArrowUpCircleIcon } from "lucide-react";

import { api } from "@/lib/api";

/**
 * Top-bar update notice: check once on page load and show when a newer release is available.
 * Clicking it opens the Version and Updates card in Settings.
 *
 * The backend caches GitHub results for 30 minutes, so checking on each mount is safe.
 * Unauthenticated GitHub API access is limited to 60 requests per hour per IP; without the
 * backend cache, opening a few tabs could exhaust the quota.
 *
 * Failures are silent here. The top bar is not the place for errors; Settings shows the reason
 * when the user explicitly checks for updates.
 */
export function UpdateBadge() {
  const [latest, setLatest] = React.useState("");

  React.useEffect(() => {
    let alive = true;
    api
      .checkUpdate()
      .then((r) => {
        // has_update already checks that version numbers can be compared; development builds
        // will not show this notice.
        if (alive && r.has_update && r.latest) setLatest(r.latest.replace(/^v(?=\d)/, ""));
      })
      .catch(() => {
        // Stay silent: network errors and GitHub rate limits should not trigger a top-bar error.
      });
    return () => {
      alive = false;
    };
  }, []);

  if (!latest) return null;

  return (
    <Link
      href="/system/settings"
      title={`Version ${latest} is available. Click to update.`}
      className="inline-flex items-center gap-1.5 rounded-full bg-primary px-2.5 py-1 font-medium text-primary-foreground text-xs transition-opacity hover:opacity-90"
    >
      {/* The pulsing indicator helps this notice stand out among the other top-bar items. */}
      <span className="relative flex size-1.5">
        <span className="absolute inline-flex size-full animate-ping rounded-full bg-primary-foreground opacity-75" />
        <span className="relative inline-flex size-1.5 rounded-full bg-primary-foreground" />
      </span>
      <ArrowUpCircleIcon className="size-3.5" />
      <span className="hidden sm:inline">Version {latest}</span>
      <span className="sm:hidden">Update</span>
    </Link>
  );
}
