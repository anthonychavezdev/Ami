import { Stack, useLocalSearchParams } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, KeyboardAvoidingView, Platform, StyleSheet, Text, View } from 'react-native';

import { Composer } from '@/components/Composer';
import { Empty, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { clock } from '@/lib/time';
import type { Message, User } from '@/lib/types';
import { socket } from '@/lib/ws';
import { colors, radius, spacing } from '@/theme';

export default function ChatScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const otherId = Number(id);
  const { user: me } = useAuth();
  const [other, setOther] = useState<User | null>(null);
  const [messages, setMessages] = useState<Message[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([api.user(otherId), api.messages(otherId)])
      .then(([u, m]) => {
        setOther(u);
        setMessages(m.slice().reverse()); // newest first for the inverted list
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [otherId]);

  useEffect(
    () =>
      socket.on('message', (e) => {
        const m: Message = e.message;
        if (m.sender_id !== otherId && m.recipient_id !== otherId) return;
        setMessages((prev) => (prev && !prev.some((x) => x.id === m.id) ? [m, ...prev] : prev));
      }),
    [otherId],
  );

  const loadOlder = useCallback(async () => {
    if (!messages?.length) return;
    const older = await api.messages(otherId, messages[messages.length - 1].id);
    if (older.length) setMessages((prev) => [...(prev ?? []), ...older.slice().reverse()]);
  }, [messages, otherId]);

  if (error) return <Empty text={error} />;
  if (!messages || !other) return <Loading />;

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1 }} keyboardVerticalOffset={90}>
      <Stack.Screen options={{ title: other.display_name }} />
      <FlatList
        data={messages}
        inverted
        keyExtractor={(m) => String(m.id)}
        renderItem={({ item }) => {
          const mine = item.sender_id === me?.id;
          return (
            <View style={[styles.bubbleRow, mine && { justifyContent: 'flex-end' }]}>
              <View style={[styles.bubble, mine ? styles.mine : styles.theirs]}>
                <Text style={[styles.text, mine && { color: '#fff' }]}>{item.body}</Text>
                <Text style={[styles.time, mine && { color: 'rgba(255,255,255,0.7)' }]}>{clock(item.created_at)}</Text>
              </View>
            </View>
          );
        }}
        onEndReached={loadOlder}
        onEndReachedThreshold={0.3}
        ListEmptyComponent={<Text style={styles.empty}>Say hi to {other.display_name} 👋</Text>}
        contentContainerStyle={{ padding: spacing.lg, flexGrow: 1 }}
      />
      <Composer placeholder="Message" buttonTitle="Send" multiline={false} compact onSubmit={async (body) => void (await api.sendMessage(otherId, body))} />
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  bubbleRow: { flexDirection: 'row', marginBottom: spacing.sm },
  bubble: { maxWidth: '80%', paddingHorizontal: spacing.md, paddingVertical: spacing.sm, borderRadius: radius.lg },
  mine: { backgroundColor: colors.accent, borderBottomRightRadius: 4 },
  theirs: { backgroundColor: colors.card, borderBottomLeftRadius: 4, borderWidth: 1, borderColor: colors.border },
  text: { fontSize: 16, color: colors.text },
  time: { fontSize: 11, color: colors.muted, marginTop: 2, alignSelf: 'flex-end' },
  empty: { color: colors.muted, textAlign: 'center', transform: [{ scaleY: -1 }] },
});
