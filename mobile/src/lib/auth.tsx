import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

import { api, setAuthToken } from './api';
import type { User } from './types';
import { setupNotifications, teardownNotifications } from './notifications';
import { storage } from './storage';
import { unread } from './unread';
import { socket } from './ws';

const TOKEN_KEY = 'ami.token';

interface AuthContextValue {
  user: User | null;
  ready: boolean;
  login: (username: string, password: string) => Promise<void>;
  register: (username: string, password: string, displayName: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [ready, setReady] = useState(false);

  const establish = useCallback(async (token: string, u: User) => {
    setAuthToken(token);
    await storage.set(TOKEN_KEY, token);
    unread.setUser(u.id);
    socket.connect(token);
    setUser(u);
    void setupNotifications();
  }, []);

  useEffect(() => {
    (async () => {
      try {
        const token = await storage.get(TOKEN_KEY);
        if (token) {
          setAuthToken(token);
          const u = await api.me();
          unread.setUser(u.id);
          socket.connect(token);
          setUser(u);
          void setupNotifications();
        }
      } catch {
        setAuthToken(null);
        await storage.remove(TOKEN_KEY).catch(() => {});
      } finally {
        setReady(true);
      }
    })();
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      ready,
      login: async (username, password) => {
        const r = await api.login(username, password);
        await establish(r.token, r.user);
      },
      register: async (username, password, displayName) => {
        const r = await api.register(username, password, displayName);
        await establish(r.token, r.user);
      },
      logout: async () => {
        await teardownNotifications().catch(() => {});
        await api.logout().catch(() => {});
        unread.setUser(null);
        socket.disconnect();
        setAuthToken(null);
        await storage.remove(TOKEN_KEY).catch(() => {});
        setUser(null);
      },
    }),
    [user, ready, establish],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth outside AuthProvider');
  return ctx;
}
