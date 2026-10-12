import { Feather } from '@expo/vector-icons';
import React, { useCallback, useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

// Browser: a small menu at the pointer, opened by right-click or a ⋯ button.
// Rendered through a portal so lists and cards never clip it.
export type MenuItem = { label: string; icon: React.ComponentProps<typeof Feather>['name']; danger?: boolean; onPress: () => void };

type State = { items: MenuItem[]; x: number; y: number };
const WIDTH = 160;
const ITEM = 36;

export function useContextMenu(): { open: (items: MenuItem[], x: number, y: number) => void; close: () => void; menu: React.ReactNode } {
  const [state, setState] = useState<State | null>(null);
  const open = useCallback((items: MenuItem[], x: number, y: number) => {
    if (!items.length) return;
    const h = 8 + items.length * ITEM;
    setState({ items, x: Math.max(4, Math.min(x, window.innerWidth - WIDTH - 4)), y: Math.max(4, Math.min(y, window.innerHeight - h - 4)) });
  }, []);
  const close = useCallback(() => setState(null), []);
  useEffect(() => {
    if (!state) return;
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && close();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [state, close]);
  return { open, close, menu: state ? createPortal(<Menu state={state} close={close} />, document.body) : null };
}

export function contextMenuProps(open: (x: number, y: number) => void): Record<string, unknown> {
  return {
    onContextMenu: (e: { preventDefault: () => void; clientX: number; clientY: number }) => {
      e.preventDefault();
      open(e.clientX, e.clientY);
    },
  };
}

export function pressPoint(e: unknown): { x: number; y: number } {
  const ne = ((e as { nativeEvent?: unknown })?.nativeEvent ?? {}) as { clientX?: number; pageX?: number; clientY?: number; pageY?: number };
  return { x: ne.clientX ?? ne.pageX ?? 0, y: ne.clientY ?? ne.pageY ?? 0 };
}

const layer = { position: 'fixed', left: 0, top: 0, right: 0, bottom: 0, zIndex: 1000 } as const;

function Menu({ state, close }: { state: State; close: () => void }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  return (
    <View style={layer as never}>
      <Pressable style={StyleSheet.absoluteFill} onPress={close} accessibilityLabel="Close menu" />
      <View style={[styles.menu, { left: state.x, top: state.y }]}>
        {state.items.map((it) => (
          <Pressable
            key={it.label}
            onPress={() => {
              close();
              it.onPress();
            }}
            style={(s) => [styles.item, (s.pressed || (s as { hovered?: boolean }).hovered) && { backgroundColor: colors.bg }]}
            accessibilityLabel={it.label}
          >
            <Feather name={it.icon} size={14} color={it.danger ? colors.danger : colors.text} />
            <Text style={[styles.text, it.danger && { color: colors.danger }]}>{it.label}</Text>
          </Pressable>
        ))}
      </View>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  menu: { position: 'absolute', width: WIDTH, backgroundColor: colors.card, borderRadius: radius.md, borderWidth: 1, borderColor: colors.border, paddingVertical: 4, shadowColor: '#000', shadowOpacity: 0.2, shadowRadius: 8, shadowOffset: { width: 0, height: 4 } },
  item: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, paddingHorizontal: spacing.md, height: ITEM },
  text: { color: colors.text, fontSize: 14 },
});
