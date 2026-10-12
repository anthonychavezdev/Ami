import { useRouter } from 'expo-router';
import React, { useCallback, useState } from 'react';
import { FlatList, RefreshControl } from 'react-native';

import { ConversationRow } from '@/components/ConversationRow';
import { Empty, Fab, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import type { Conversation } from '@/lib/types';
import { useLiveLoad } from '@/lib/useLiveLoad';
import { useTheme } from '@/lib/theme';

// Events after which the list is reloaded.
const REFRESH_ON = ['message', 'message.updated', 'message.deleted', 'messages.read', 'group.added'] as const;

// Direct chats and group chats in one list, most recent first.
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
  useLiveLoad(load, REFRESH_ON);

  if (!convos) return <Loading />;
  return (
    <>
      <FlatList
        data={convos}
        keyExtractor={(c) => (c.group ? `g${c.group.id}` : `u${c.user!.id}`)}
        renderItem={({ item }) => <ConversationRow conversation={item} me={me?.id} />}
        ListEmptyComponent={
          <Empty text="No conversations yet. Open a friend's profile to message them, or start a group chat with the + button." />
        }
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={() => {
              setRefreshing(true);
              void load();
            }}
            tintColor={colors.accent}
          />
        }
        contentContainerStyle={{ flexGrow: 1, paddingBottom: 90 }}
      />
      <Fab label="New group chat" onPress={() => router.push('/new-group-chat')} />
    </>
  );
}
