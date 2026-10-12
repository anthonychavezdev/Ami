import { Feather } from '@expo/vector-icons';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, KeyboardAvoidingView, Platform, Pressable, StyleSheet, Text, View } from 'react-native';

import { AttachmentList } from '@/components/Attachments';
import { Composer } from '@/components/Composer';
import { Empty, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { useCall } from '@/lib/call';
import { clock } from '@/lib/time';
import type { Message, User } from '@/lib/types';
import { socket } from '@/lib/ws';
import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

export default function ChatScreen() {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  const otherId = Number(id);
  const { user: me } = useAuth();
  const { call } = useCall();
  const router = useRouter();
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

  const startCall = (video: boolean) => {
    if (!other) return;
    void call.startCall(other, video);
    router.push('/call');
  };

  if (error) return <Empty text={error} />;
  if (!messages || !other) return <Loading />;

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1 }} keyboardVerticalOffset={90}>
      <Stack.Screen
        options={{
          title: other.display_name,
          headerRight: () => (
            <View style={{ flexDirection: 'row', gap: spacing.md }}>
              <Pressable onPress={() => startCall(false)} hitSlop={8} accessibilityLabel="Voice call">
                <Feather name="phone" size={22} color={colors.accent} />
              </Pressable>
              <Pressable onPress={() => startCall(true)} hitSlop={8} accessibilityLabel="Video call">
                <Feather name="video" size={22} color={colors.accent} />
              </Pressable>
            </View>
          ),
        }}
      />
      <FlatList
        data={messages}
        inverted
        keyExtractor={(m) => String(m.id)}
        renderItem={({ item }) => {
          const mine = item.sender_id === me?.id;
          const hasAttachments = item.attachments?.length > 0;
          return (
            <View style={[styles.bubbleRow, mine && { justifyContent: 'flex-end' }]}>
              <View style={[styles.bubble, mine ? styles.mine : styles.theirs, hasAttachments && styles.wide]}>
                {item.body ? <Text style={[styles.text, mine && { color: '#fff' }]}>{item.body}</Text> : null}
                {hasAttachments && <AttachmentList items={item.attachments} style={{ marginTop: item.body ? spacing.sm : 0 }} />}
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
      <Composer placeholder="Message" buttonTitle="Send" multiline={false} compact media onSubmit={async (body, attachments) => void (await api.sendMessage(otherId, body, attachments))} />
    </KeyboardAvoidingView>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  bubbleRow: { flexDirection: 'row', marginBottom: spacing.sm },
  bubble: { maxWidth: '80%', paddingHorizontal: spacing.md, paddingVertical: spacing.sm, borderRadius: radius.lg },
  wide: { width: '80%' },
  mine: { backgroundColor: colors.accent, borderBottomRightRadius: 4 },
  theirs: { backgroundColor: colors.card, borderBottomLeftRadius: 4, borderWidth: 1, borderColor: colors.border },
  text: { fontSize: 16, color: colors.text },
  time: { fontSize: 11, color: colors.muted, marginTop: 2, alignSelf: 'flex-end' },
  empty: { color: colors.muted, textAlign: 'center', transform: [{ scaleY: -1 }] },
});
