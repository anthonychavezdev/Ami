import { Feather } from '@expo/vector-icons';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { AppState, FlatList, KeyboardAvoidingView, Platform, Pressable, StyleSheet, Text, View } from 'react-native';

import { AttachmentList } from '@/components/Attachments';
import { Composer } from '@/components/Composer';
import { contextMenuProps, pressPoint, useContextMenu, type MenuItem } from '@/components/ContextMenu';
import { Empty, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { useCall } from '@/lib/call';
import { confirmAction } from '@/lib/confirm';
import { clock } from '@/lib/time';
import { toast } from '@/lib/toast';
import type { Message, User } from '@/lib/types';
import { unread } from '@/lib/unread';
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
  const [editing, setEditing] = useState<Message | null>(null); // my message being edited
  const [selected, setSelected] = useState<number | null>(null); // my message showing Edit / Delete
  const [hovered, setHovered] = useState<number | null>(null); // web: bubble under the mouse (shows the dots)
  const { open: openMenu, menu } = useContextMenu();
  const list = useRef<FlatList<Message>>(null);
  const loaded = messages !== null;

  // react-native-web flips inverted lists with a CSS transform, which also
  // flips the mouse wheel; undo that.
  useEffect(() => {
    if (Platform.OS !== 'web' || !loaded) return;
    const node = list.current?.getScrollableNode?.() as HTMLElement | undefined;
    if (!node) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      node.scrollTop -= e.deltaY;
    };
    node.addEventListener('wheel', onWheel, { passive: false });
    return () => node.removeEventListener('wheel', onWheel);
  }, [loaded]);

  useEffect(() => {
    Promise.all([api.user(otherId), api.messages(otherId)])
      .then(([u, m]) => {
        setOther(u);
        setMessages(m.slice().reverse()); // newest first for the inverted list
        void api.markRead(otherId);
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [otherId]);

  // While this chat is open, new messages in it are read, not notified.
  useEffect(() => {
    unread.setActiveChat(otherId);
    const sub = AppState.addEventListener('change', (s) => {
      if (s === 'active') void api.markRead(otherId);
    });
    return () => {
      unread.setActiveChat(null);
      sub.remove();
    };
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
  useEffect(
    () =>
      socket.on('message.updated', (e) => {
        const m: Message = e.message;
        setMessages((prev) => prev?.map((x) => (x.id === m.id ? m : x)) ?? prev);
      }),
    [],
  );
  useEffect(() => socket.on('message.deleted', (e) => setMessages((prev) => prev?.filter((x) => x.id !== e.id) ?? prev)), []);

  // Web menu (right-click or ⋯) for my messages.
  const messageMenu = (m: Message): MenuItem[] => [
    { label: 'Edit', icon: 'edit-2', onPress: () => setEditing(m) },
    { label: 'Delete', icon: 'trash-2', danger: true, onPress: () => remove(m) },
  ];

  const remove = (m: Message) =>
    confirmAction('Delete message?', m.body || 'picture', 'Delete', async () => {
      try {
        await api.deleteMessage(otherId, m.id);
        setMessages((prev) => prev?.filter((x) => x.id !== m.id) ?? prev);
        setSelected(null);
        if (editing?.id === m.id) setEditing(null);
      } catch (e) {
        toast.error(e instanceof Error ? e.message : String(e));
      }
    });

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
      <View style={{ flex: 1 }}>
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
          ref={list}
          data={messages}
          inverted
          keyExtractor={(m) => String(m.id)}
          renderItem={({ item }) => {
            const mine = item.sender_id === me?.id;
            const hasAttachments = item.attachments?.length > 0;
            return (
              <View style={[styles.bubbleRow, mine && { alignItems: 'flex-end' }]}>
                <Pressable
                  {...(mine ? contextMenuProps((x, y) => openMenu(messageMenu(item), x, y)) : {})}
                  onLongPress={mine ? () => setSelected((s) => (s === item.id ? null : item.id)) : undefined}
                  delayLongPress={400}
                  onHoverIn={mine ? () => setHovered(item.id) : undefined}
                  onHoverOut={mine ? () => setHovered((h) => (h === item.id ? null : h)) : undefined}
                  style={[styles.bubble, mine ? styles.mine : styles.theirs, hasAttachments && styles.wide]}
                >
                  {mine && Platform.OS === 'web' && hovered === item.id && (
                    <Pressable onPress={(e) => openMenu(messageMenu(item), pressPoint(e).x, pressPoint(e).y)} hitSlop={6} style={styles.dots} accessibilityLabel="Message options">
                      <Feather name="more-horizontal" size={18} color={colors.muted} />
                    </Pressable>
                  )}
                  {item.edited_at ? (
                    <View style={styles.edited} accessibilityLabel="edited">
                      <Feather name="edit-2" size={10} color={mine ? 'rgba(255,255,255,0.75)' : colors.muted} />
                      <Text style={[styles.editedText, mine && { color: 'rgba(255,255,255,0.75)' }]}>edited</Text>
                    </View>
                  ) : null}
                  {item.body ? <Text style={[styles.text, mine && { color: '#fff' }]}>{item.body}</Text> : null}
                  {hasAttachments && <AttachmentList items={item.attachments} style={{ marginTop: item.body ? spacing.sm : 0 }} />}
                  <Text style={[styles.time, mine && { color: 'rgba(255,255,255,0.7)' }]}>{clock(item.created_at)}</Text>
                </Pressable>
                {selected === item.id && (
                  <View style={styles.msgActions}>
                    <Pressable
                      onPress={() => {
                        setSelected(null);
                        setEditing(item);
                      }}
                      hitSlop={6}
                      style={styles.msgAction}
                      accessibilityLabel="Edit message"
                    >
                      <Feather name="edit-2" size={13} color={colors.muted} />
                      <Text style={styles.msgActionText}>Edit</Text>
                    </Pressable>
                    <Pressable onPress={() => remove(item)} hitSlop={6} style={styles.msgAction} accessibilityLabel="Delete message">
                      <Feather name="trash-2" size={13} color={colors.muted} />
                      <Text style={styles.msgActionText}>Delete</Text>
                    </Pressable>
                  </View>
                )}
              </View>
            );
          }}
          onEndReached={loadOlder}
          onEndReachedThreshold={0.3}
          ListEmptyComponent={<Text style={styles.empty}>Say hi to {other.display_name} 👋</Text>}
          contentContainerStyle={{ padding: spacing.lg, flexGrow: 1 }}
        />
        {editing && (
          <View style={styles.editBar}>
            <Feather name="edit-2" size={14} color={colors.accent} />
            <Text style={styles.editText} numberOfLines={1}>
              Editing: {editing.body || 'picture'}
            </Text>
            <Pressable onPress={() => setEditing(null)} hitSlop={8} accessibilityLabel="Cancel editing">
              <Feather name="x" size={18} color={colors.muted} />
            </Pressable>
          </View>
        )}
        <Composer
          key={editing?.id ?? 'new'}
          placeholder="Message"
          buttonTitle={editing ? 'Save' : 'Send'}
          initialText={editing?.body}
          autoFocus={!!editing}
          multiline={false}
          compact
          media={!editing}
          onSubmit={async (body, attachments) => {
            if (editing) {
              await api.updateMessage(otherId, editing.id, body);
              setEditing(null);
            } else {
              await api.sendMessage(otherId, body, attachments);
            }
          }}
        />
        {menu}
      </View>
    </KeyboardAvoidingView>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  bubbleRow: { marginBottom: spacing.sm, alignItems: 'flex-start' },
  msgActions: { flexDirection: 'row', gap: spacing.lg, marginTop: 4, paddingHorizontal: spacing.sm },
  dots: { position: 'absolute', left: -30, top: 0, bottom: 0, width: 26, alignItems: 'center', justifyContent: 'center' },
  msgAction: { flexDirection: 'row', alignItems: 'center', gap: 4 },
  msgActionText: { color: colors.muted, fontSize: 12, fontWeight: '600' },
  bubble: { maxWidth: '80%', paddingHorizontal: spacing.md, paddingVertical: spacing.sm, borderRadius: radius.lg },
  wide: { width: '80%' },
  mine: { backgroundColor: colors.accent, borderBottomRightRadius: 4 },
  theirs: { backgroundColor: colors.card, borderBottomLeftRadius: 4, borderWidth: 1, borderColor: colors.border },
  text: { fontSize: 16, color: colors.text },
  time: { fontSize: 11, color: colors.muted, marginTop: 2, alignSelf: 'flex-end' },
  empty: { color: colors.muted, textAlign: 'center', transform: [{ scaleY: -1 }] },
  edited: { flexDirection: 'row', alignItems: 'center', gap: 3, marginBottom: 2 },
  editedText: { fontSize: 10, color: colors.muted },
  editBar: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, paddingHorizontal: spacing.lg, paddingVertical: spacing.sm, backgroundColor: colors.card, borderTopWidth: 1, borderTopColor: colors.border },
  editText: { flex: 1, color: colors.text, fontSize: 13 },
});
