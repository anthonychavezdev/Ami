import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

import { api, setAuthToken } from './api';
import type { Me } from './types';
import { setupNotifications, teardownNotifications } from './notifications';
import { startRealtime } from './realtime';
import { storage } from './storage';
import { unread } from './unread';
import { socket } from './ws';

const TOKEN_KEY = 'ami.token';

interface AuthContextValue {
  user: Me | null;
  ready: boolean;
  update: (patch: Partial<Me>) => void; // local settings changes (e.g. appear offline)
  login: (username: string, password: string) => Promise<void>;
  register: (username: string, password: string, displayName: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<Me | null>(null);
  const [ready, setReady] = useState(false);

  // Wires everything that depends on being signed in: the API token, the
  // WebSocket and its listeners, the unread badge and OS notifications.
  const signIn = useCallback((token: string, u: Me) => {
    setAuthToken(token);
    unread.setUser(u.id);
    startRealtime();
    socket.connect(token);
    setUser(u);
    void setupNotifications();
  }, []);

  const signOut = useCallback(async () => {
    unread.setUser(null);
    socket.disconnect();
    setAuthToken(null);
    await storage.remove(TOKEN_KEY).catch(() => {});
    setUser(null);
  }, []);

  // On start, resume the stored session if it is still valid.
  useEffect(() => {
    storage
      .get(TOKEN_KEY)
      .then(async (token) => {
        if (!token) return;
        setAuthToken(token);
        signIn(token, await api.me());
      })
      .catch(signOut)
      .finally(() => setReady(true));
  }, [signIn, signOut]);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      ready,
      update: (patch) => setUser((u) => (u ? { ...u, ...patch } : u)),
      login: async (username, password) => {
        const r = await api.login(username, password);
        await storage.set(TOKEN_KEY, r.token);
        signIn(r.token, r.user);
      },
      register: async (username, password, displayName) => {
        const r = await api.register(username, password, displayName);
        await storage.set(TOKEN_KEY, r.token);
        signIn(r.token, r.user);
      },
      logout: async () => {
        await teardownNotifications().catch(() => {});
        await api.logout().catch(() => {});
        await signOut();
      },
    }),
    [user, ready, signIn, signOut],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth outside AuthProvider');
  return ctx;
}
