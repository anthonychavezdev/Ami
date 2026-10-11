import { useRouter } from 'expo-router';
import React from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { Avatar } from './ui';
import type { User } from '@/lib/types';
import { useStyles } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

export function UserRow({ user, subtitle, right, onPress }: { user: User; subtitle?: string; right?: React.ReactNode; onPress?: () => void }) {
  const styles = useStyles(makeStyles);
  const router = useRouter();
  return (
    <Pressable onPress={onPress ?? (() => router.push(`/user/${user.id}`))} style={({ pressed }) => [styles.row, pressed && { opacity: 0.7 }]}>
      <Avatar user={user} size={44} />
      <View style={{ flex: 1 }}>
        <Text style={styles.name}>{user.display_name}</Text>
        <Text style={styles.sub} numberOfLines={1}>
          {subtitle ?? `@${user.username}`}
        </Text>
      </View>
      {right}
    </Pressable>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.lg, paddingVertical: spacing.md, backgroundColor: colors.card, borderBottomWidth: 1, borderBottomColor: colors.border },
  name: { fontWeight: '700', color: colors.text, fontSize: 15 },
  sub: { color: colors.muted, fontSize: 13 },
});
