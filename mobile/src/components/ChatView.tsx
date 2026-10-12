import { Feather } from '@expo/vector-icons';
import { Stack, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { AppState, FlatList, KeyboardAvoidingView, Platform, Pressable, StyleSheet, Text, View } from 'react-native';

import { AttachmentList } from './Attachments';
import { Composer } from './Composer';
import { contextMenuProps, pressPoint, useContextMenu, type MenuItem } from './ContextMenu';
import { Avatar, Empty, Loading } from './ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { useCall } from '@/lib/call';
import { confirmAction } from '@/lib/confirm';
import { clock } from '@/lib/time';
import { toast } from '@/lib/toast';
import type { ChatTarget, Group, GroupMember, Message, User } from '@/lib/types';
import { unread } from '@/lib/unread';
import { socket } from '@/lib/ws';
import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

// One chat screen for both kinds of chat: with a friend (kind 'user') or in a
// group (kind 'group'). Group chats name the author of each message, let the
// admin delete anything, and show the group call going on, if any.
export function ChatView({ kind, id }: ChatTarget) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const chat = useMemo<ChatTarget>(() => ({ kind, id }), [kind, id]);
  const isGroup = kind === 'group';
  const { user: me } = useAuth();
  const { state: callState, call } = useCall();
  const router = useRouter();
  const [other, setOther] = useState<User | null>(null);
  const [group, setGroup] = useState<Group | null>(null);
  const [members, setMembers] = useState<Map<number, GroupMember>>(new Map());
  const [inCall, setInCall] = useState<number[]>([]); // members in the group call right now
  const [messages, setMessages] = useState<Message[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<Message | null>(null); // my message being edited
  const [selected, setSelected] = useState<number | null>(null); // message showing Edit / Delete
  const [hovered, setHovered] = useState<number | null>(null); // web: bubble under the mouse (shows the dots)
  const { open: openMenu, menu } = useContextMenu();
  const list = useRef<FlatList<Message>>(null);
  const loaded = messages !== null;
  const room = isGroup ? `g:${id}` : null;
  const inThisCall = room !== null && callState.room === room && callState.status !== 'idle' && callState.status !== 'incoming';

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
    const who = isGroup
      ? api.group(id).then((g) => {
          setGroup(g.group);
          setMembers(new Map(g.members.map((m) => [m.id, m])));
          setInCall(g.call);
        })
      : api.user(id).then(setOther);
    Promise.all([who, api.messages(chat)])
      .then(([, m]) => {
        setMessages(m.slice().reverse()); // newest first for the inverted list
        void api.markRead(chat);
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [chat, id, isGroup]);

  // While this chat is open, new messages in it are read, not notified.
  useEffect(() => {
    unread.setActiveChat(chat);
    const sub = AppState.addEventListener('change', (s) => {
      if (s === 'active') void api.markRead(chat);
    });
    return () => {
      unread.setActiveChat(null);
      sub.remove();
    };
  }, [chat]);

  const inChat = useCallback(
    (m: Pick<Message, 'group_id' | 'sender_id' | 'recipient_id'>) => (isGroup ? m.group_id === id : m.group_id == null && (m.sender_id === id || m.recipient_id === id)),
    [isGroup, id],
  );
  useEffect(() => {
    const offs = [
      socket.on('message', (e) => {
        const m: Message = e.message;
        if (!inChat(m)) return;
        setMessages((prev) => (prev && !prev.some((x) => x.id === m.id) ? [m, ...prev] : prev));
      }),
      socket.on('message.updated', (e) => {
        const m: Message = e.message;
        if (inChat(m)) setMessages((prev) => prev?.map((x) => (x.id === m.id ? m : x)) ?? prev);
      }),
      socket.on('message.deleted', (e) => {
        if (inChat({ group_id: e.group_id ?? null, sender_id: e.sender_id, recipient_id: e.recipient_id ?? null })) setMessages((prev) => prev?.filter((x) => x.id !== e.id) ?? prev);
      }),
    ];
    return () => offs.forEach((off) => off());
  }, [inChat]);

  // Who is in the group call, for the banner.
  useEffect(() => {
    if (!room) return;
    const offs = [
      socket.on('call.ring', (e) => e.room === room && setInCall([e.from_user.id])),
      socket.on('call.peer_joined', (e) => e.room === room && setInCall((ids) => (ids.includes(e.user.id) ? ids : [...ids, e.user.id]))),
      socket.on('call.peer_left', (e) => e.room === room && setInCall((ids) => ids.filter((x) => x !== e.user_id))),
      socket.on('call.ended', (e) => e.room === room && setInCall([])),
    ];
    return () => offs.forEach((off) => off());
  }, [room]);

  const isAdmin = group?.my_role === 'admin';
  const canDelete = (m: Message) => m.sender_id === me?.id || (isGroup && isAdmin);

  // Web menu (right-click or ⋯).
  const messageMenu = (m: Message): MenuItem[] => [
    ...(m.sender_id === me?.id ? [{ label: 'Edit', icon: 'edit-2', onPress: () => setEditing(m) } as MenuItem] : []),
    { label: 'Delete', icon: 'trash-2', danger: true, onPress: () => remove(m) },
  ];

  const remove = (m: Message) =>
    confirmAction('Delete message?', m.body || 'picture', 'Delete', async () => {
      try {
        await api.deleteMessage(chat, m.id);
        setMessages((prev) => prev?.filter((x) => x.id !== m.id) ?? prev);
        setSelected(null);
        if (editing?.id === m.id) setEditing(null);
      } catch (e) {
        toast.error(e instanceof Error ? e.message : String(e));
      }
    });

  const loadOlder = useCallback(async () => {
    if (!messages?.length) return;
    const older = await api.messages(chat, messages[messages.length - 1].id);
    if (older.length) setMessages((prev) => [...(prev ?? []), ...older.slice().reverse()]);
  }, [messages, chat]);

  const startCall = (video: boolean) => {
    if (isGroup) {
      if (!group) return;
      if (!inThisCall) void call.joinGroupCall({ id: group.id, name: group.name }, video);
    } else {
      if (!other) return;
      void call.startCall(other, video);
    }
    router.push('/call');
  };

  if (error) return <Empty text={error} />;
  if (!messages || (isGroup ? !group : !other)) return <Loading />;
  const title = isGroup ? group!.name : other!.display_name;
  const othersInCall = inCall.filter((x) => x !== me?.id);

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1 }} keyboardVerticalOffset={90}>
      <View style={{ flex: 1 }}>
        <Stack.Screen
          options={{
            title,
            headerRight: () => (
              <View style={{ flexDirection: 'row', gap: spacing.md }}>
                <Pressable onPress={() => startCall(false)} hitSlop={8} accessibilityLabel={isGroup ? 'Group voice call' : 'Voice call'}>
                  <Feather name="phone" size={22} color={colors.accent} />
                </Pressable>
                <Pressable onPress={() => startCall(true)} hitSlop={8} accessibilityLabel={isGroup ? 'Group video call' : 'Video call'}>
                  <Feather name="video" size={22} color={colors.accent} />
                </Pressable>
                {isGroup && (
                  <Pressable onPress={() => router.push(`/group/${id}`)} hitSlop={8} accessibilityLabel="Group details">
                    <Feather name="info" size={22} color={colors.accent} />
                  </Pressable>
                )}
              </View>
            ),
          }}
        />
        {isGroup && (inThisCall || othersInCall.length > 0) && (
          <Pressable onPress={() => startCall(false)} style={styles.callBanner} accessibilityLabel={inThisCall ? 'Return to call' : 'Join call'}>
            <Feather name="phone-call" size={16} color="#fff" />
            <Text style={styles.callBannerText}>
              {inThisCall ? 'You are in this call' : `Call in progress · ${othersInCall.length} in the call`}
            </Text>
            <Text style={styles.callBannerAction}>{inThisCall ? 'Return' : 'Join'}</Text>
          </Pressable>
        )}
        <FlatList
          ref={list}
          data={messages}
          inverted
          keyExtractor={(m) => String(m.id)}
          renderItem={({ item }) => {
            const mine = item.sender_id === me?.id;
            const hasAttachments = item.attachments?.length > 0;
            const sender: User | undefined = mine ? undefined : item.sender ?? members.get(item.sender_id);
            const showSender = isGroup && !mine;
            const manage = canDelete(item);
            return (
              <View style={[styles.bubbleRow, mine && { alignItems: 'flex-end' }]}>
                <View style={[styles.line, mine && { justifyContent: 'flex-end' }]}>
                  {showSender && <View style={styles.senderAvatar}>{sender ? <Avatar user={sender} size={28} status={false} /> : null}</View>}
                  <Pressable
                    {...(manage ? contextMenuProps((x, y) => openMenu(messageMenu(item), x, y)) : {})}
                    onLongPress={manage ? () => setSelected((s) => (s === item.id ? null : item.id)) : undefined}
                    delayLongPress={400}
                    onHoverIn={manage ? () => setHovered(item.id) : undefined}
                    onHoverOut={manage ? () => setHovered((h) => (h === item.id ? null : h)) : undefined}
                    style={[styles.bubble, mine ? styles.mine : styles.theirs, hasAttachments && styles.wide]}
                  >
                    {manage && Platform.OS === 'web' && hovered === item.id && (
                      <Pressable onPress={(e) => openMenu(messageMenu(item), pressPoint(e).x, pressPoint(e).y)} hitSlop={6} style={[styles.dots, mine ? { left: -30 } : { right: -30 }]} accessibilityLabel="Message options">
                        <Feather name="more-horizontal" size={18} color={colors.muted} />
                      </Pressable>
                    )}
                    {showSender && <Text style={styles.senderName}>{sender?.display_name ?? 'Former member'}</Text>}
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
                </View>
                {selected === item.id && (
                  <View style={[styles.msgActions, showSender && { marginLeft: 36 }]}>
                    {mine && (
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
                    )}
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
          ListEmptyComponent={<Text style={styles.empty}>{isGroup ? `Say hi to ${title} 👋` : `Say hi to ${title} 👋`}</Text>}
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
              await api.updateMessage(chat, editing.id, body);
              setEditing(null);
            } else {
              await api.sendMessage(chat, body, attachments);
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
  line: { flexDirection: 'row', alignItems: 'flex-end', gap: spacing.sm, maxWidth: '100%' },
  senderAvatar: { width: 28, height: 28, marginBottom: 2 },
  senderName: { fontSize: 12, fontWeight: '700', color: colors.accent, marginBottom: 2 },
  msgActions: { flexDirection: 'row', gap: spacing.lg, marginTop: 4, paddingHorizontal: spacing.sm },
  dots: { position: 'absolute', top: 0, bottom: 0, width: 26, alignItems: 'center', justifyContent: 'center' },
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
  callBanner: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, paddingHorizontal: spacing.lg, paddingVertical: spacing.sm, backgroundColor: colors.success },
  callBannerText: { flex: 1, color: '#fff', fontWeight: '600' },
  callBannerAction: { color: '#fff', fontWeight: '800' },
});
