import { Feather } from '@expo/vector-icons';
// expo-router vendors React Navigation; this is the context its native stack provides.
import { HeaderHeightContext } from 'expo-router/build/react-navigation/elements';
import React, { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { Animated, Easing, Pressable, StyleSheet, Text } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

export type ToastKind = 'info' | 'success' | 'error';
type Toast = { id: number; message: string; kind: ToastKind };
type Listener = (t: Toast) => void;

let seq = 0;
const listeners = new Set<Listener>();

// toast.show('Call declined'): a small banner at the top that goes away by itself.
export const toast = {
  show(message: string, kind: ToastKind = 'info') {
    const t = { id: ++seq, message, kind };
    listeners.forEach((l) => l(t));
  },
  error(message: string) {
    toast.show(message, 'error');
  },
  success(message: string) {
    toast.show(message, 'success');
  },
};

const DURATION = 2200;

// Rendered inside every screen (via the Stack's screenLayout): on Android a
// pushed native-stack screen paints over root-level overlays, so the banner
// has to live in the topmost screen itself. Each instance shows the banner.
export function ToastHost() {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const insets = useSafeAreaInsets();
  const headerHeight = useContext(HeaderHeightContext) ?? 0; // > 0 when the screen has a native header above its content
  const top = (headerHeight > 0 ? 0 : insets.top) + spacing.sm;
  const [current, setCurrent] = useState<Toast | null>(null);
  // A fresh value per banner, so the previous fade-out cannot leave this one invisible.
  const opacity = useMemo(() => new Animated.Value(1), [current]);

  const hide = useCallback(() => {
    Animated.timing(opacity, { toValue: 0, duration: 200, easing: Easing.out(Easing.quad), useNativeDriver: false }).start(({ finished }) => finished && setCurrent(null));
  }, [opacity]);

  useEffect(() => {
    const on = (t: Toast) => setCurrent(t);
    listeners.add(on);
    return () => void listeners.delete(on);
  }, []);

  // Shown as soon as it renders (a screen under a closing modal may render it
  // late), then fades out. No entry transform: a transform animated while the
  // screen is frozen never reaches the native view.
  useEffect(() => {
    if (!current) return;
    const timer = setTimeout(hide, DURATION);
    return () => clearTimeout(timer);
  }, [current, hide]);

  if (!current) return null;
  const icon = ({ info: 'info', success: 'check-circle', error: 'alert-circle' } as const)[current.kind];
  const tint = { info: colors.accent, success: colors.success, error: colors.danger }[current.kind];
  return (
    <Animated.View pointerEvents="box-none" style={[styles.wrap, { top, opacity }]}>
      <Pressable onPress={hide} style={[styles.banner, { borderLeftColor: tint }]}>
        <Feather name={icon} size={18} color={tint} />
        <Text style={styles.text} numberOfLines={2}>
          {current.message}
        </Text>
      </Pressable>
    </Animated.View>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  wrap: { position: 'absolute', left: spacing.lg, right: spacing.lg, zIndex: 1000, elevation: 20, alignItems: 'center' },
  banner: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    width: '100%',
    maxWidth: 520,
    backgroundColor: colors.card,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.border,
    borderLeftWidth: 4,
    paddingVertical: 10,
    paddingHorizontal: spacing.md,
    shadowColor: '#000',
    shadowOpacity: 0.15,
    shadowRadius: 10,
    shadowOffset: { width: 0, height: 4 },
    elevation: 6,
  },
  text: { color: colors.text, fontWeight: '600', flexShrink: 1 },
});
