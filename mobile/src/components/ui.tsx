import { Feather } from '@expo/vector-icons';
import { Image } from 'expo-image';
import { useHeaderHeight } from 'expo-router/build/react-navigation/elements';
import React from 'react';
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Pressable,
  StyleProp,
  StyleSheet,
  Text,
  TextInput,
  TextInputProps,
  TextStyle,
  View,
  ViewStyle,
} from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { API_URL } from '@/lib/config';
import { COLUMN_WIDTH, useWide } from '@/lib/layout';
import { useOnline } from '@/lib/presence';
import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';
import type { AvatarCrop, User } from '@/lib/types';

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
  return (
    <TextInput ref={ref} placeholderTextColor={colors.muted} keyboardAppearance={scheme} {...props} style={[styles.input, props.style]} />
  );
}

// A hue from a name, so the same name always gets the same colour.
export const nameHue = (name: string) => [...name].reduce((h, c) => (h * 31 + c.charCodeAt(0)) % 360, 0);

// Places the picture so that its chosen square fills a circle of `size`.
function cropStyle(crop: AvatarCrop | undefined, size: number) {
  if (!crop?.w) return { width: size, height: size };
  const width = size / crop.w;
  const height = width / crop.aspect;
  return { position: 'absolute' as const, left: -crop.x * width, top: -crop.y * height, width, height };
}

// Initials on a colored disc, with a presence dot (green online, gray offline)
// at the bottom right like Teams. `status={false}` hides the dot.
export function Avatar({
  user,
  size = 40,
  status = true,
}: {
  user: Pick<User, 'display_name' | 'username'> & { id?: number; online?: boolean; avatar_url?: string; avatar_crop?: AvatarCrop };
  size?: number;
  status?: boolean;
}) {
  const { colors } = useTheme();
  const online = useOnline(user.id, user.online);
  const initials = (user.display_name || user.username).trim().slice(0, 2).toUpperCase();
  const dot = Math.max(10, Math.round(size * 0.3));
  return (
    <View
      style={{
        width: size,
        height: size,
        borderRadius: size / 2,
        backgroundColor: `hsl(${nameHue(user.username)}, 55%, 45%)`,
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      {user.avatar_url ? (
        <View style={{ width: size, height: size, borderRadius: size / 2, overflow: 'hidden' }}>
          <Image
            source={{ uri: API_URL + user.avatar_url }}
            style={cropStyle(user.avatar_crop, size)}
            contentFit={user.avatar_crop?.w ? 'fill' : 'cover'}
            accessibilityLabel="Profile picture"
          />
        </View>
      ) : (
        <Text style={{ color: '#fff', fontWeight: '700', fontSize: size * 0.38 }}>{initials}</Text>
      )}
      {status && !!user.id && (
        <View
          accessibilityLabel={online ? 'Online' : 'Offline'}
          style={{
            position: 'absolute',
            right: -1,
            bottom: -1,
            width: dot,
            height: dot,
            borderRadius: dot / 2,
            backgroundColor: online ? '#22c55e' : colors.muted,
            borderWidth: 2,
            borderColor: colors.card,
          }}
        />
      )}
    </View>
  );
}

// A group's icon: a disc coloured from its name.
export function GroupAvatar({ name, size = 40 }: { name: string; size?: number }) {
  return (
    <View
      accessibilityLabel="Group"
      style={{
        width: size,
        height: size,
        borderRadius: size / 2,
        backgroundColor: `hsl(${nameHue(name)}, 45%, 45%)`,
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <Feather name="users" size={Math.round(size * 0.5)} color="#fff" />
    </View>
  );
}

// A list row: something on the left, a title with a subtitle, something on the right.
export function Row({
  left,
  title,
  subtitle,
  right,
  onPress,
  selected,
}: {
  left: React.ReactNode;
  title: string;
  subtitle?: string;
  right?: React.ReactNode;
  onPress?: () => void;
  selected?: boolean; // the open item of a list beside its detail (wide Messages view)
}) {
  const styles = useStyles(makeStyles);
  return (
    <Pressable onPress={onPress} style={({ pressed }) => [styles.row, selected && styles.rowSelected, pressed && { opacity: 0.7 }]}>
      {left}
      <View style={{ flex: 1 }}>
        <Text style={styles.rowTitle}>{title}</Text>
        {subtitle !== undefined && (
          <Text style={styles.rowSub} numberOfLines={1}>
            {subtitle}
          </Text>
        )}
      </View>
      {right}
    </Pressable>
  );
}

// The round button in a tab's bottom corner.
export function Fab({ label, onPress }: { label: string; onPress: () => void }) {
  const styles = useStyles(makeStyles);
  return (
    <Pressable style={styles.fab} onPress={onPress} accessibilityLabel={label}>
      <Feather name="plus" size={28} color="#fff" />
    </Pressable>
  );
}

// A small uppercase heading above a form field, a list section or a group of settings.
export function SectionLabel({ children, style }: { children: React.ReactNode; style?: StyleProp<TextStyle> }) {
  const styles = useStyles(makeStyles);
  return <Text style={[styles.sectionLabel, style]}>{children}</Text>;
}

// On wide windows a screen sits in a column of readable width instead of
// stretching across the window; on phones it is the whole screen.
export function WideColumn({ children, width = COLUMN_WIDTH }: { children: React.ReactNode; width?: number }) {
  const wide = useWide();
  if (!wide) return <>{children}</>;
  return (
    <View style={{ flex: 1, alignItems: 'center' }}>
      <View style={{ flex: 1, width: '100%', maxWidth: width }}>{children}</View>
    </View>
  );
}

// A screen with a text box at the bottom (chat, comments, a form): the box
// stays above the keyboard and, with the keyboard away, above the home
// indicator. Padding on both platforms: Android draws edge to edge, so the
// window no longer shrinks for the keyboard by itself. The offset is the
// header's real height, which varies by phone.
// A list in a pane of its own beside what it opened: how Messages, Groups and
// Friends lay out on a wide window.
export function Panes({ list, children }: { list: React.ReactNode; children: React.ReactNode }) {
  const styles = useStyles(makeStyles);
  return (
    <View style={styles.panes}>
      <View style={styles.listPane}>{list}</View>
      <View style={styles.detailPane}>{children}</View>
    </View>
  );
}

export function KeyboardScreen({ children }: { children: React.ReactNode }) {
  const headerHeight = useHeaderHeight();
  return (
    <SafeAreaView edges={['bottom']} style={{ flex: 1 }}>
      <KeyboardAvoidingView behavior="padding" keyboardVerticalOffset={headerHeight} style={{ flex: 1 }}>
        {children}
      </KeyboardAvoidingView>
    </SafeAreaView>
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

export function Segmented<T extends string>({
  options,
  value,
  onChange,
}: {
  options: { key: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
}) {
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

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    panes: {
      flex: 1,
      flexDirection: 'row',
      margin: spacing.lg,
      marginTop: 0,
      borderWidth: 1,
      borderColor: colors.border,
      borderRadius: 12,
      overflow: 'hidden',
    },
    listPane: { width: 340, borderRightWidth: 1, borderRightColor: colors.border, backgroundColor: colors.card },
    detailPane: { flex: 1 },
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
    row: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: spacing.md,
      paddingHorizontal: spacing.lg,
      paddingVertical: spacing.md,
      backgroundColor: colors.card,
      borderBottomWidth: 1,
      borderBottomColor: colors.border,
    },
    rowSelected: { backgroundColor: colors.accentSoft },
    rowTitle: { fontWeight: '700', color: colors.text, fontSize: 15 },
    rowSub: { color: colors.muted, fontSize: 13 },
    fab: {
      position: 'absolute',
      right: spacing.xl,
      bottom: spacing.xl,
      width: 56,
      height: 56,
      borderRadius: 28,
      backgroundColor: colors.accent,
      alignItems: 'center',
      justifyContent: 'center',
      elevation: 6,
      shadowColor: '#000',
      shadowOpacity: 0.2,
      shadowRadius: 8,
      shadowOffset: { width: 0, height: 4 },
    },
    sectionLabel: { color: colors.muted, fontWeight: '700', fontSize: 13, textTransform: 'uppercase' },
    center: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: spacing.xl },
    muted: { color: colors.muted, textAlign: 'center' },
    error: { backgroundColor: colors.danger, padding: spacing.md },
    card: { backgroundColor: colors.card, borderRadius: radius.md, padding: spacing.lg, borderWidth: 1, borderColor: colors.border },
    segment: {
      flexDirection: 'row',
      backgroundColor: colors.accentSoft,
      borderRadius: radius.pill,
      padding: 3,
      margin: spacing.lg,
      marginBottom: spacing.sm,
    },
    segmentItem: { flex: 1, paddingVertical: 8, borderRadius: radius.pill, alignItems: 'center' },
    segmentActive: { backgroundColor: colors.accent },
    segmentText: { color: colors.accent, fontWeight: '600' },
  });
