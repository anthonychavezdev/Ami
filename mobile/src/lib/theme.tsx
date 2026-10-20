import * as SystemUI from 'expo-system-ui';
import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { Appearance, StyleSheet, useColorScheme } from 'react-native';

import { darkColors, lightColors, type ThemeColors } from '@/theme';
import { storage } from './storage';

export type ThemePreference = 'light' | 'dark' | 'system';
export type ColorScheme = 'light' | 'dark';

const PREFERENCE_KEY = 'ami.theme';

interface ThemeValue {
  preference: ThemePreference;
  setPreference: (p: ThemePreference) => void;
  scheme: ColorScheme; // the resolved scheme actually in use
  colors: ThemeColors;
}

const ThemeContext = createContext<ThemeValue | null>(null);

function isPreference(v: unknown): v is ThemePreference {
  return v === 'light' || v === 'dark' || v === 'system';
}

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const system = useColorScheme();
  const [preference, setPreferenceState] = useState<ThemePreference>('system');

  useEffect(() => {
    storage
      .get(PREFERENCE_KEY)
      .then((v) => {
        if (isPreference(v)) setPreferenceState(v);
      })
      .catch(() => {});
  }, []);

  const setPreference = useCallback((p: ThemePreference) => {
    setPreferenceState(p);
    storage.set(PREFERENCE_KEY, p).catch(() => {});
  }, []);

  const scheme: ColorScheme = preference === 'system' ? (system === 'dark' ? 'dark' : 'light') : preference;
  const colors = scheme === 'dark' ? darkColors : lightColors;

  // Root view behind every screen, so transitions never flash the wrong color.
  useEffect(() => {
    SystemUI.setBackgroundColorAsync(colors.bg).catch(() => {});
  }, [colors.bg]);

  // The OS draws part of the screen itself: the status bar, the native
  // header's buttons, keyboards, alerts. Those follow the system appearance,
  // so a chosen theme is also declared to the OS; "system" hands it back.
  useEffect(() => {
    Appearance.setColorScheme?.(preference === 'system' ? 'unspecified' : preference);
  }, [preference]);

  const value = useMemo<ThemeValue>(() => ({ preference, setPreference, scheme, colors }), [preference, setPreference, scheme, colors]);
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme outside ThemeProvider');
  return ctx;
}

// Memoizes a StyleSheet built from the active palette:
//   const styles = useStyles(makeStyles);   // makeStyles = (colors: ThemeColors) => StyleSheet.create({...})
export function useStyles<T extends StyleSheet.NamedStyles<T>>(make: (colors: ThemeColors) => T): T {
  const { colors } = useTheme();
  return useMemo(() => make(colors), [make, colors]);
}
