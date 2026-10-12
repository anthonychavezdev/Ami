import { useFocusEffect, useRouter } from 'expo-router';
import React, { useCallback, useState } from 'react';
import { FlatList, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { UserRow } from '@/components/UserRow';
import { Empty, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { messagePreview } from '@/lib/messages';
import { timeAgo } from '@/lib/time';
import type { Conversation } from '@/lib/types';
import { socket } from '@/lib/ws';
import { useStyles, useTheme } from '@/lib/theme';
import { type ThemeColors, radius, spacing } from '@/theme';

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
      const offs = [socket.on('message', () => void load()), socket.on('message.updated', () => void load()), socket.on('messages.read', () => void load())];
      return () => offs.forEach((off) => off());
    }, [load]),
  );

  if (!convos) return <Loading />;
  return (
    <FlatList
      data={convos}
      keyExtractor={(c) => String(c.user.id)}
      renderItem={({ item }) => (
        <UserRow
          user={item.user}
          subtitle={`${item.last_message.sender_id === me?.id ? 'You: ' : ''}${messagePreview(item.last_message)} · ${timeAgo(item.last_message.created_at)}`}
          right={
            item.unread > 0 ? (
              <View style={styles.badge} accessibilityLabel={`${item.unread} unread`}>
                <Text style={styles.badgeText}>{item.unread > 99 ? '99+' : item.unread}</Text>
              </View>
            ) : undefined
          }
          onPress={() => router.push(`/chat/${item.user.id}`)}
        />
      )}
      ListEmptyComponent={<Empty text="No conversations yet. Open a friend's profile to message them." />}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { setRefreshing(true); void load(); }} tintColor={colors.accent} />}
      contentContainerStyle={{ flexGrow: 1 }}
    />
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  badge: { minWidth: 22, height: 22, borderRadius: radius.pill, paddingHorizontal: 6, backgroundColor: colors.danger, alignItems: 'center', justifyContent: 'center' },
  badgeText: { color: '#fff', fontSize: 12, fontWeight: '700' },
});
