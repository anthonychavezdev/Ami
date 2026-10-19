import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useState } from 'react';
import { FlatList, RefreshControl } from 'react-native';

import { ChatView } from '@/components/ChatView';
import { ConversationRow } from '@/components/ConversationRow';
import { Empty, Fab, Loading, Panes } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { useWide } from '@/lib/layout';
import { chatKey, parseChatKey } from '@/lib/messages';
import type { Conversation } from '@/lib/types';
import { useLiveLoad } from '@/lib/useLiveLoad';
import { useTheme } from '@/lib/theme';

// Events after which the list is reloaded.
const REFRESH_ON = ['message', 'message.updated', 'message.deleted', 'messages.read', 'group.added'] as const;

// Direct chats and group chats in one list, most recent first. On a wide
// window the open chat sits beside the list, named by ?chat= so a reload or a
// link (/chat/... redirects here) lands on it.
export default function MessagesScreen() {
  const { colors } = useTheme();
  const { user: me } = useAuth();
  const router = useRouter();
  const wide = useWide();
  const { chat: chatParam } = useLocalSearchParams<{ chat?: string }>();
  const open = wide ? parseChatKey(chatParam) : null;
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
  const list = (
    <FlatList
      data={convos}
      keyExtractor={(c) => (c.group ? `g${c.group.id}` : `u${c.user!.id}`)}
      renderItem={({ item }) => (
        <ConversationRow
          conversation={item}
          me={me?.id}
          selected={!!open && chatKey(open) === (item.group ? `g:${item.group.id}` : `u:${item.user!.id}`)}
          onOpen={wide ? (chat) => router.setParams({ chat: chatKey(chat) }) : undefined}
        />
      )}
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
  );
  const fab = <Fab label="New group chat" onPress={() => router.push('/new-group-chat')} />;
  if (!wide) {
    return (
      <>
        {list}
        {fab}
      </>
    );
  }
  return (
    <Panes
      list={
        <>
          {list}
          {fab}
        </>
      }
    >
      {open ? <ChatView key={chatKey(open)} kind={open.kind} id={open.id} embedded /> : <Empty text="Pick a conversation." />}
    </Panes>
  );
}
