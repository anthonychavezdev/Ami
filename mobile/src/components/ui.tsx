import { Feather } from '@expo/vector-icons';
import React from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, TextInput, TextInputProps, View, ViewStyle } from 'react-native';

import { useOnline } from '@/lib/presence';
import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';
import type { User } from '@/lib/types';

export function Button({
  title,
  onPress,
  variant = 'primary',
  disabled,
  loading,
  style,
  small,
  icon,
}: {
  title: string;
  onPress?: () => void;
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  disabled?: boolean;
  loading?: boolean;
  style?: ViewStyle;
  small?: boolean;
  icon?: React.ComponentProps<typeof Feather>['name'];
}) {
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const bg = { primary: colors.accent, secondary: colors.accentSoft, danger: colors.danger, ghost: 'transparent' }[variant];
  const fg = { primary: '#fff', secondary: colors.accent, danger: '#fff', ghost: colors.accent }[variant];
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled || loading}
      style={({ pressed }) => [
        styles.btn,
        small && styles.btnSmall,
        { backgroundColor: bg, opacity: disabled ? 0.5 : pressed ? 0.8 : 1 },
        style,
      ]}
    >
      {loading ? (
        <ActivityIndicator color={fg} />
      ) : (
        <View style={styles.btnContent}>
          {icon && <Feather name={icon} size={small ? 15 : 18} color={fg} />}
          <Text style={[styles.btnText, small && styles.btnTextSmall, { color: fg }]}>{title}</Text>
        </View>
      )}
    </Pressable>
  );
}

export function Input({ ref, ...props }: TextInputProps & { ref?: React.Ref<TextInput> }) {
  const { colors, scheme } = useTheme();
  const styles = useStyles(makeStyles);
  return <TextInput ref={ref} placeholderTextColor={colors.muted} keyboardAppearance={scheme} {...props} style={[styles.input, props.style]} />;
}

// Initials on a colored disc, with a presence dot (green online, gray offline)
// at the bottom right like Teams. `status={false}` hides the dot.
export function Avatar({ user, size = 40, status = true }: { user: Pick<User, 'display_name' | 'username'> & { id?: number; online?: boolean }; size?: number; status?: boolean }) {
  const { colors } = useTheme();
  const online = useOnline(user.id, user.online);
  const initials = (user.display_name || user.username).trim().slice(0, 2).toUpperCase();
  const hue = [...user.username].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 0);
  const dot = Math.max(10, Math.round(size * 0.3));
  return (
    <View style={{ width: size, height: size, borderRadius: size / 2, backgroundColor: `hsl(${hue}, 55%, 45%)`, alignItems: 'center', justifyContent: 'center' }}>
      <Text style={{ color: '#fff', fontWeight: '700', fontSize: size * 0.38 }}>{initials}</Text>
      {status && !!user.id && (
        <View
          accessibilityLabel={online ? 'Online' : 'Offline'}
          style={{ position: 'absolute', right: -1, bottom: -1, width: dot, height: dot, borderRadius: dot / 2, backgroundColor: online ? '#22c55e' : colors.muted, borderWidth: 2, borderColor: colors.card }}
        />
      )}
    </View>
  );
}

export function Loading() {
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  return (
    <View style={styles.center}>
      <ActivityIndicator color={colors.accent} />
    </View>
  );
}

export function Empty({ text }: { text: string }) {
  const styles = useStyles(makeStyles);
  return (
    <View style={styles.center}>
      <Text style={styles.muted}>{text}</Text>
    </View>
  );
}

export function ErrorBanner({ error, onDismiss }: { error: string | null; onDismiss?: () => void }) {
  const styles = useStyles(makeStyles);
  if (!error) return null;
  return (
    <Pressable onPress={onDismiss} style={styles.error}>
      <Text style={{ color: '#fff' }}>{error}</Text>
    </Pressable>
  );
}

export function Card({ children, style }: { children: React.ReactNode; style?: ViewStyle }) {
  const styles = useStyles(makeStyles);
  return <View style={[styles.card, style]}>{children}</View>;
}

export function Segmented<T extends string>({ options, value, onChange }: { options: { key: T; label: string }[]; value: T; onChange: (v: T) => void }) {
  const styles = useStyles(makeStyles);
  return (
    <View style={styles.segment}>
      {options.map((o) => (
        <Pressable key={o.key} onPress={() => onChange(o.key)} style={[styles.segmentItem, value === o.key && styles.segmentActive]}>
          <Text style={[styles.segmentText, value === o.key && { color: '#fff' }]}>{o.label}</Text>
        </Pressable>
      ))}
    </View>
  );
}

export function useErrorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  btn: { paddingVertical: 12, paddingHorizontal: 18, borderRadius: radius.md, alignItems: 'center', justifyContent: 'center' },
  btnSmall: { paddingVertical: 7, paddingHorizontal: 12, borderRadius: radius.pill },
  btnContent: { flexDirection: 'row', alignItems: 'center', gap: 6 },
  btnText: { fontWeight: '600', fontSize: 16 },
  btnTextSmall: { fontSize: 14 },
  input: {
    backgroundColor: colors.card,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    paddingVertical: 10,
    fontSize: 16,
    color: colors.text,
  },
  center: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: spacing.xl },
  muted: { color: colors.muted, textAlign: 'center' },
  error: { backgroundColor: colors.danger, padding: spacing.md },
  card: { backgroundColor: colors.card, borderRadius: radius.md, padding: spacing.lg, borderWidth: 1, borderColor: colors.border },
  segment: { flexDirection: 'row', backgroundColor: colors.accentSoft, borderRadius: radius.pill, padding: 3, margin: spacing.lg, marginBottom: spacing.sm },
  segmentItem: { flex: 1, paddingVertical: 8, borderRadius: radius.pill, alignItems: 'center' },
  segmentActive: { backgroundColor: colors.accent },
  segmentText: { color: colors.accent, fontWeight: '600' },
});
