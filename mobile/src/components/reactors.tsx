import { useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { Avatar } from './ui';
import { api } from '@/lib/api';
import { errorMessage } from '@/lib/errors';
import { useStyles } from '@/lib/theme';
import type { Post, ReactionGroup, User } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

// What the two ReactionSummary twins share: the pills, and who is behind them.

// Reactions sorted by count, biggest first.
export const sortedReactions = (post: Post) => Object.entries(post.reactions).sort((a, b) => b[1] - a[1]);

// Who reacted to a post, loaded while `when` holds (the hover or the sheet).
export function useReactors(postId: string, when: boolean): { groups: ReactionGroup[] | null; error: string | null } {
  const [loaded, setLoaded] = useState<{ postId: string; groups?: ReactionGroup[]; error?: string } | null>(null);
  useEffect(() => {
    if (!when) return;
    let live = true;
    api.postReactions(postId).then(
      (groups) => live && setLoaded({ postId, groups }),
      (e) => live && setLoaded({ postId, error: errorMessage(e) }),
    );
    return () => {
      live = false;
    };
  }, [postId, when]);
  if (!when || loaded?.postId !== postId) return { groups: null, error: null };
  return { groups: loaded.groups ?? null, error: loaded.error ?? null };
}

// The names behind one emoji in a line: "Ana García, Luis Martínez and 3 others".
export function reactorNames(users: User[], max = 6): string {
  const names = users.map((u) => u.display_name);
  if (names.length <= max) return names.length <= 1 ? names.join('') : `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`;
  return `${names.slice(0, max).join(', ')} and ${names.length - max} others`;
}

// One person who reacted; tapping opens their profile.
export function ReactorRow({ user, onOpen }: { user: User; onOpen?: () => void }) {
  const styles = useStyles(makeStyles);
  const router = useRouter();
  return (
    <Pressable
      onPress={() => {
        onOpen?.();
        router.push(`/user/${user.id}`);
      }}
      style={({ pressed }) => [styles.row, pressed && { opacity: 0.6 }]}
      accessibilityLabel={user.display_name}
    >
      <Avatar user={user} size={36} />
      <View style={{ flex: 1 }}>
        <Text style={styles.name} numberOfLines={1}>
          {user.display_name}
        </Text>
        <Text style={styles.username} numberOfLines={1}>
          @{user.username}
        </Text>
      </View>
    </Pressable>
  );
}

export const makePillStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    reactions: { flexDirection: 'row', flexWrap: 'wrap', gap: 6, marginTop: spacing.md },
    pill: {
      backgroundColor: colors.bg,
      borderRadius: radius.pill,
      paddingHorizontal: 8,
      paddingVertical: 3,
      borderWidth: 1,
      borderColor: colors.border,
    },
    mine: { borderColor: colors.accent, backgroundColor: colors.accentSoft },
    pillText: { fontSize: 14 },
  });

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    row: { flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingVertical: spacing.sm },
    name: { color: colors.text, fontWeight: '600' },
    username: { color: colors.muted, fontSize: 13 },
  });
