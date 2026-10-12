import { Feather } from '@expo/vector-icons';
// expo-router vendors React Navigation; this is the context its native stack provides.
import { HeaderHeightContext } from 'expo-router/build/react-navigation/elements';
import React, { useCallback, useContext, useEffect, useState } from 'react';
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
  const insets = useSafeAreaInsets();
  const headerHeight = useContext(HeaderHeightContext) ?? 0; // > 0 when the screen has a native header above its content
  const top = (headerHeight > 0 ? 0 : insets.top) + spacing.sm;
  const [current, setCurrent] = useState<Toast | null>(null);

  useEffect(() => {
    const on = (t: Toast) => setCurrent(t);
    listeners.add(on);
    return () => void listeners.delete(on);
  }, []);

  if (!current) return null;
  // Keyed by id: each banner is a fresh component with its own opacity, so the
  // previous fade-out cannot leave this one invisible.
  return <Banner key={current.id} toast={current} top={top} onHidden={() => setCurrent(null)} />;
}

// Shown as soon as it renders (a screen under a closing modal may render it
// late), then fades out. No entry transform: a transform animated while the
// screen is frozen never reaches the native view.
function Banner({ toast: t, top, onHidden }: { toast: Toast; top: number; onHidden: () => void }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const [opacity] = useState(() => new Animated.Value(1));

  const hide = useCallback(() => {
    Animated.timing(opacity, { toValue: 0, duration: 200, easing: Easing.out(Easing.quad), useNativeDriver: false }).start(
      ({ finished }) => finished && onHidden(),
    );
  }, [opacity, onHidden]);

  useEffect(() => {
    const timer = setTimeout(hide, DURATION);
    return () => clearTimeout(timer);
  }, [hide]);

  const icon = ({ info: 'info', success: 'check-circle', error: 'alert-circle' } as const)[t.kind];
  const tint = { info: colors.accent, success: colors.success, error: colors.danger }[t.kind];
  return (
    <Animated.View pointerEvents="box-none" style={[styles.wrap, { top, opacity }]}>
      <Pressable onPress={hide} style={[styles.banner, { borderLeftColor: tint }]}>
        <Feather name={icon} size={18} color={tint} />
        <Text style={styles.text} numberOfLines={2}>
          {t.message}
        </Text>
      </Pressable>
    </Animated.View>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
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
