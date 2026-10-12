import { Feather } from '@expo/vector-icons';
import { useFocusEffect, useRouter } from 'expo-router';
import React, { useCallback, useState } from 'react';
import { FlatList, Pressable, RefreshControl, StyleSheet } from 'react-native';

import { ConversationRow } from '@/components/ConversationRow';
import { Empty, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import type { Conversation } from '@/lib/types';
import { socket } from '@/lib/ws';
import { useStyles, useTheme } from '@/lib/theme';
import { type ThemeColors, spacing } from '@/theme';

// Events after which the list is reloaded.
const REFRESH_ON = ['message', 'message.updated', 'message.deleted', 'messages.read', 'group.added'];

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
      const offs = REFRESH_ON.map((type) => socket.on(type, () => void load()));
      return () => offs.forEach((off) => off());
    }, [load]),
  );

  if (!convos) return <Loading />;
  return (
    <>
      <FlatList
        data={convos}
        keyExtractor={(c) => (c.group ? `g${c.group.id}` : `u${c.user!.id}`)}
        renderItem={({ item }) => <ConversationRow conversation={item} me={me?.id} />}
        ListEmptyComponent={<Empty text="No conversations yet. Open a friend's profile to message them, or start a group chat with the + button." />}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { setRefreshing(true); void load(); }} tintColor={colors.accent} />}
        contentContainerStyle={{ flexGrow: 1, paddingBottom: 90 }}
      />
      <Pressable style={styles.fab} onPress={() => router.push('/new-group-chat')} accessibilityLabel="New group chat">
        <Feather name="plus" size={28} color="#fff" />
      </Pressable>
    </>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  fab: { position: 'absolute', right: spacing.xl, bottom: spacing.xl, width: 56, height: 56, borderRadius: 28, backgroundColor: colors.accent, alignItems: 'center', justifyContent: 'center', elevation: 6, shadowColor: '#000', shadowOpacity: 0.2, shadowRadius: 8, shadowOffset: { width: 0, height: 4 } },
});
