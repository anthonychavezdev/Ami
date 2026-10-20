import { Feather } from '@expo/vector-icons';
import React, { useState } from 'react';
import { Modal, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { type ReactionSummaryProps, ReactorRow, makePillStyles, sortedReactions, useReactors } from './reactors';
import { Loading } from './ui';
import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

// The reaction pills under a post or comment. Holding one opens a sheet that
// shows who reacted, emoji by emoji, starting on the one held.
// (ReactionSummary.web.tsx shows them on hover instead.)
export function ReactionSummary(props: ReactionSummaryProps) {
  const pills = useStyles(makePillStyles);
  const [held, setHeld] = useState<string | null>(null);
  const reactions = sortedReactions(props.reactions);
  if (reactions.length === 0) return null;
  return (
    <>
      <View style={[pills.reactions, props.compact && pills.compact]}>
        {reactions.map(([emoji, count]) => (
          <Pressable
            key={emoji}
            onLongPress={() => setHeld(emoji)}
            delayLongPress={300}
            accessibilityLabel={`${emoji} ${count}`}
            accessibilityHint="Hold to see who reacted"
            style={[pills.pill, emoji === props.myReaction && pills.mine]}
          >
            <Text style={pills.pillText}>
              {emoji} {count}
            </Text>
          </Pressable>
        ))}
      </View>
      <ReactorsSheet {...props} emoji={held} onClose={() => setHeld(null)} />
    </>
  );
}

// A bottom sheet: one tab per emoji, the people behind the selected one.
function ReactorsSheet({ id, load, emoji, onClose }: ReactionSummaryProps & { emoji: string | null; onClose: () => void }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const insets = useSafeAreaInsets();
  const [picked, setPicked] = useState<string | null>(null);
  const { groups, error } = useReactors(id, emoji !== null, load);
  const current = picked ?? emoji;
  const group = groups?.find((g) => g.emoji === current) ?? groups?.[0];
  const close = () => {
    setPicked(null);
    onClose();
  };
  return (
    <Modal visible={emoji !== null} transparent animationType="slide" onRequestClose={close}>
      <Pressable style={styles.backdrop} onPress={close} accessibilityLabel="Close" />
      <View style={[styles.sheet, { paddingBottom: insets.bottom + spacing.lg }]}>
        <View style={styles.header}>
          <Text style={styles.title}>Reactions</Text>
          <Pressable onPress={close} hitSlop={10} accessibilityLabel="Close">
            <Feather name="x" size={20} color={colors.muted} />
          </Pressable>
        </View>
        {groups && (
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.tabs}>
            {groups.map((g) => (
              <Pressable
                key={g.emoji}
                onPress={() => setPicked(g.emoji)}
                style={[styles.tab, g.emoji === group?.emoji && styles.tabOn]}
                accessibilityLabel={`${g.emoji} ${g.users.length}`}
              >
                <Text style={styles.tabText}>
                  {g.emoji} {g.users.length}
                </Text>
              </Pressable>
            ))}
          </ScrollView>
        )}
        <ScrollView style={styles.list}>
          {error && <Text style={styles.error}>{error}</Text>}
          {!groups && !error && <Loading />}
          {group?.users.map((u) => (
            <ReactorRow key={u.id} user={u} onOpen={close} />
          ))}
          {groups && !group && <Text style={styles.error}>Nobody has reacted yet.</Text>}
        </ScrollView>
      </View>
    </Modal>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    backdrop: { flex: 1, backgroundColor: 'rgba(0,0,0,0.4)' },
    sheet: {
      backgroundColor: colors.card,
      borderTopLeftRadius: radius.lg,
      borderTopRightRadius: radius.lg,
      paddingHorizontal: spacing.lg,
      paddingTop: spacing.md,
      maxHeight: '70%',
    },
    header: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginBottom: spacing.sm },
    title: { color: colors.text, fontSize: 17, fontWeight: '700' },
    tabs: { gap: spacing.sm, paddingVertical: spacing.sm },
    tab: {
      paddingHorizontal: 10,
      paddingVertical: 6,
      borderRadius: radius.pill,
      borderWidth: 1,
      borderColor: colors.border,
      backgroundColor: colors.bg,
    },
    tabOn: { borderColor: colors.accent, backgroundColor: colors.accentSoft },
    tabText: { color: colors.text, fontSize: 14 },
    list: { marginTop: spacing.sm },
    error: { color: colors.muted, paddingVertical: spacing.md },
  });
