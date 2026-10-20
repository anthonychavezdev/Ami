import { useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { Avatar } from './ui';
import { errorMessage } from '@/lib/errors';
import { useStyles } from '@/lib/theme';
import type { ReactionGroup, User } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

// What the two ReactionSummary twins share: the pills, and who is behind them.
// A summary is given the reactions of a post or a comment and the API call
// that lists the people.
export type ReactionSummaryProps = {
  id: string;
  reactions: Record<string, number>;
  myReaction: string;
  load: (id: string) => Promise<ReactionGroup[]>;
  compact?: boolean; // under a comment: tighter spacing
};

// Reactions sorted by count, biggest first.
export const sortedReactions = (reactions: Record<string, number>) => Object.entries(reactions).sort((a, b) => b[1] - a[1]);

// Who reacted, loaded while `when` holds (the hover or the sheet).
export function useReactors(
  id: string,
  when: boolean,
  load: (id: string) => Promise<ReactionGroup[]>,
): { groups: ReactionGroup[] | null; error: string | null } {
  const [loaded, setLoaded] = useState<{ id: string; groups?: ReactionGroup[]; error?: string } | null>(null);
  useEffect(() => {
    if (!when) return;
    let live = true;
    load(id).then(
      (groups) => live && setLoaded({ id, groups }),
      (e) => live && setLoaded({ id, error: errorMessage(e) }),
    );
    return () => {
      live = false;
    };
  }, [id, when, load]);
  if (!when || loaded?.id !== id) return { groups: null, error: null };
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
    compact: { marginTop: 6 },
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
