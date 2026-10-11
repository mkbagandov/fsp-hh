// Сессия пользователя: кто вошёл, роль, согласия; вход/выход; восстановление по refresh-cookie при загрузке.
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, getSession, mayHaveSession, onSession, refreshSession, setSession, unwrap, type Schemas } from "./client";

export type User = Schemas["UserInfo"];
export type Consent = Schemas["ConsentInfo"];
export type Role = User["role"];

type SessionState = {
  ready: boolean;
  user: User | null;
  consents: Consent[];
  reload: () => Promise<void>;
  applySession: (s: { access_token: string; expires_at: string; user: User }) => Promise<void>;
  logout: () => Promise<void>;
};

const Ctx = createContext<SessionState | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false);
  const [user, setUser] = useState<User | null>(null);
  const [consents, setConsents] = useState<Consent[]>([]);
  const qc = useQueryClient();

  const loadMe = useCallback(async () => {
    if (!getSession()) {
      setUser(null);
      setConsents([]);
      return;
    }
    try {
      const me = unwrap(await api.GET("/auth/me"));
      setUser(me.user);
      setConsents(me.consents ?? []);
    } catch {
      setUser(null);
      setConsents([]);
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (mayHaveSession()) await refreshSession();
      if (!cancelled) {
        await loadMe();
        setReady(true);
      }
    })();
    const off = onSession((s) => {
      if (!s) {
        setUser(null);
        setConsents([]);
      }
    });
    return () => {
      cancelled = true;
      off();
    };
  }, [loadMe]);

  const value = useMemo<SessionState>(
    () => ({
      ready,
      user,
      consents,
      reload: loadMe,
      applySession: async (s) => {
        setSession({ accessToken: s.access_token, expiresAt: Date.parse(s.expires_at) });
        setUser(s.user);
        await loadMe();
      },
      logout: async () => {
        try {
          await api.POST("/auth/logout");
        } finally {
          setSession(null);
          setUser(null);
          setConsents([]);
          qc.clear();
        }
      },
    }),
    [ready, user, consents, loadMe, qc],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSession(): SessionState {
  const v = useContext(Ctx);
  if (!v) throw new Error("useSession вне SessionProvider");
  return v;
}

export function homeFor(role: Role | undefined): string {
  switch (role) {
    case "candidate":
      return "/candidate";
    case "employer":
      return "/employer";
    case "admin":
      return "/admin";
    default:
      return "/";
  }
}
