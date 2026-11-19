"use client";

import { useEffect, useState } from "react";

import { auth, type CurrentUser } from "@/lib/auth";
import { api } from "@/lib/api";

const FALLBACK: CurrentUser = { id: "1", name: "ARTEX", username: "artex", email: "", avatar: "", role: "operator" };

export function useCurrentUser(): CurrentUser {
  const [user, setUser] = useState<CurrentUser>(FALLBACK);

  useEffect(() => {
    const u = auth.getCurrentUser();
    if (u) setUser(u);
    let active = true;
    api.account().then(({ username }) => {
      if (active) setUser({ ...(u ?? FALLBACK), name: username, username });
    }).catch(() => { /* Keep the token's display name when the backend is unavailable. */ });
    return () => { active = false; };
  }, []);

  return user;
}
