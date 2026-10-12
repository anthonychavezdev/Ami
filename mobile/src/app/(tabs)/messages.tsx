import { useFocusEffect, useRouter } from 'expo-router';
import React, { useCallback, useState } from 'react';
import { FlatList, RefreshControl } from 'react-native';

import { UserRow } from '@/components/UserRow';
import { Empty, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { timeAgo } from '@/lib/time';
import type { Conversation, Message } from '@/lib/types';
import { socket } from '@/lib/ws';
import { useTheme } from '@/lib/theme';

// Text of the last message, or what kind of attachment it was.
const preview = (m: Message) => m.body || (m.attachments?.[0]?.type === 'video' ? 'Video' : m.attachments?.[0]?.type === 'link' ? 'Link' : 'Picture');

export default function MessagesScreen() {
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
      return socket.on('message', () => void load());
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
          subtitle={`${item.last_message.sender_id === me?.id ? 'You: ' : ''}${preview(item.last_message)} · ${timeAgo(item.last_message.created_at)}`}
          onPress={() => router.push(`/chat/${item.user.id}`)}
        />
      )}
      ListEmptyComponent={<Empty text="No conversations yet. Open a friend's profile to message them." />}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { setRefreshing(true); void load(); }} tintColor={colors.accent} />}
      contentContainerStyle={{ flexGrow: 1 }}
    />
  );
}
