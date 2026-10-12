import { Feather } from '@expo/vector-icons';
import { useFocusEffect, useRouter } from 'expo-router';
import React, { useCallback, useState } from 'react';
import { FlatList, Pressable, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { UserRow } from '@/components/UserRow';
import { Empty, GroupAvatar, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { messagePreview } from '@/lib/messages';
import { timeAgo } from '@/lib/time';
import type { Conversation } from '@/lib/types';
import { socket } from '@/lib/ws';
import { useStyles, useTheme } from '@/lib/theme';
import { type ThemeColors, radius, spacing } from '@/theme';

// Direct chats and group chats in one list, most recent first.
export default function MessagesScreen() {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const { user: me } = useAuth();
  const router = useRouter();
  const [convos, setConvos] = useState<Conversation[] | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async () => {
    try {
      setConvos(await api.conversations());
    } catch {
      setConvos((c) => c ?? []);
    } finally {
      setRefreshing(false);
    }
  }, []);

  useFocusEffect(
    useCallback(() => {
      void load();
      const offs = ['message', 'message.updated', 'message.deleted', 'messages.read', 'group.added'].map((t) => socket.on(t, () => void load()));
      return () => offs.forEach((off) => off());
    }, [load]),
  );

  if (!convos) return <Loading />;

  const badge = (n: number) =>
    n > 0 ? (
      <View style={styles.badge} accessibilityLabel={`${n} unread`}>
        <Text style={styles.badgeText}>{n > 99 ? '99+' : n}</Text>
      </View>
    ) : undefined;

  return (
    <View style={{ flex: 1 }}>
      <FlatList
        data={convos}
        keyExtractor={(c) => (c.group ? `g${c.group.id}` : `u${c.user!.id}`)}
        renderItem={({ item }) => {
          const last = item.last_message;
          const when = last ? ` · ${timeAgo(last.created_at)}` : '';
          if (item.group) {
            const who = last ? (last.sender_id === me?.id ? 'You' : last.sender?.display_name ?? 'Someone') + ': ' : '';
            return (
              <Pressable onPress={() => router.push(`/chat/group/${item.group!.id}`)} style={({ pressed }) => [styles.row, pressed && { opacity: 0.7 }]}>
                <GroupAvatar name={item.group.name} size={44} />
                <View style={{ flex: 1 }}>
                  <Text style={styles.name}>{item.group.name}</Text>
                  <Text style={styles.sub} numberOfLines={1}>
                    {last ? `${who}${messagePreview(last)}${when}` : `${item.group.member_count} members · no messages yet`}
                  </Text>
                </View>
                {badge(item.unread)}
              </Pressable>
            );
          }
          return (
            <UserRow
              user={item.user!}
              subtitle={last ? `${last.sender_id === me?.id ? 'You: ' : ''}${messagePreview(last)}${when}` : ''}
              right={badge(item.unread)}
              onPress={() => router.push(`/chat/${item.user!.id}`)}
            />
          );
        }}
        ListEmptyComponent={<Empty text="No conversations yet. Open a friend's profile to message them, or start a group chat with the + button." />}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { setRefreshing(true); void load(); }} tintColor={colors.accent} />}
        contentContainerStyle={{ flexGrow: 1, paddingBottom: 90 }}
      />
      <Pressable style={styles.fab} onPress={() => router.push('/new-group-chat')} accessibilityLabel="New group chat">
        <Feather name="plus" size={28} color="#fff" />
      </Pressable>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.lg, paddingVertical: spacing.md, backgroundColor: colors.card, borderBottomWidth: 1, borderBottomColor: colors.border },
  name: { fontWeight: '700', color: colors.text, fontSize: 15 },
  sub: { color: colors.muted, fontSize: 13 },
  badge: { minWidth: 22, height: 22, borderRadius: radius.pill, paddingHorizontal: 6, backgroundColor: colors.danger, alignItems: 'center', justifyContent: 'center' },
  badgeText: { color: '#fff', fontSize: 12, fontWeight: '700' },
  fab: { position: 'absolute', right: spacing.xl, bottom: spacing.xl, width: 56, height: 56, borderRadius: 28, backgroundColor: colors.accent, alignItems: 'center', justifyContent: 'center', elevation: 6, shadowColor: '#000', shadowOpacity: 0.2, shadowRadius: 8, shadowOffset: { width: 0, height: 4 } },
});
