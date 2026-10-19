import { Feather } from '@expo/vector-icons';
import { Stack, useRouter } from 'expo-router';
import React, { useRef, useState } from 'react';
import { FlatList, Pressable, StyleSheet, Text, View } from 'react-native';

import { CallBanner } from './CallBanner';
import { Composer } from './Composer';
import { useContextMenu } from './ContextMenu';
import { EditBar } from './EditBar';
import { MessageBubble, type MessageActions } from './MessageBubble';
import { Empty, KeyboardScreen, Loading, PaneHeader } from './ui';
import { useAuth } from '@/lib/auth';
import { useCall } from '@/lib/call';
import { confirmAction } from '@/lib/confirm';
import { toast } from '@/lib/toast';
import { groupRoomKey } from '@/lib/messages';
import type { ChatTarget, Message } from '@/lib/types';
import { useChat } from '@/lib/useChat';
import { useInvertedWheel } from '@/lib/useInvertedWheel';
import { useStyles, useTheme } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';
import { errorMessage } from '@/lib/errors';

// One chat screen for both kinds of chat: with a friend (kind 'user') or in a
// group (kind 'group'). Group chats name the author of each message, let the
// admin delete anything, and show the group call going on, if any. `embedded`
// is the pane of the wide Messages view: it draws its own header instead of
// configuring the navigator's.
export function ChatView({ kind, id, embedded = false }: ChatTarget & { embedded?: boolean }) {
  const styles = useStyles(makeStyles);
  const chat = useChat(kind, id);
  const { user: me } = useAuth();
  const { state: callState, call } = useCall();
  const router = useRouter();
  const [editing, setEditing] = useState<Message | null>(null);
  const [selected, setSelected] = useState<string | null>(null); // message showing Edit / Delete (native)
  const { open: openMenu, menu } = useContextMenu();
  const list = useRef<FlatList<Message>>(null);
  useInvertedWheel(list, chat.messages !== null);

  const room = chat.isGroup ? groupRoomKey(id) : null;
  const inThisCall = room !== null && callState.room === room && callState.status !== 'idle' && callState.status !== 'incoming';
  const isAdmin = chat.group?.my_role === 'admin';

  const startCall = (video: boolean) => {
    if (chat.isGroup && chat.group && !inThisCall) void call.joinGroupCall({ id: chat.group.id, name: chat.group.name }, video);
    else if (!chat.isGroup && chat.other) void call.startCall(chat.other, video);
    router.push('/call');
  };

  const remove = (m: Message) =>
    confirmAction('Delete message?', m.body || 'picture', 'Delete', async () => {
      try {
        await chat.remove(m);
        setSelected(null);
        if (editing?.id === m.id) setEditing(null);
      } catch (e) {
        toast.error(errorMessage(e));
      }
    });

  // What the viewer may do with a message: edit their own, delete their own or (as the admin) anyone's.
  const actionsFor = (m: Message): MessageActions => {
    const mine = m.sender_id === me?.id;
    return {
      edit: mine ? () => (setSelected(null), setEditing(m)) : undefined,
      remove: mine || (chat.isGroup && isAdmin) ? () => remove(m) : undefined,
    };
  };

  if (chat.error) return <Empty text={chat.error} />;
  if (!chat.messages || (chat.isGroup ? !chat.group : !chat.other)) return <Loading />;
  const title = chat.isGroup ? chat.group!.name : chat.other!.display_name;
  const buttons = <HeaderButtons isGroup={chat.isGroup} groupId={id} onCall={startCall} />;

  return (
    <KeyboardScreen>
      <View style={{ flex: 1 }}>
        {embedded ? <PaneHeader title={title}>{buttons}</PaneHeader> : <Stack.Screen options={{ title, headerRight: () => buttons }} />}
        {chat.isGroup && (
          <CallBanner inThisCall={inThisCall} others={chat.inCall.filter((x) => x !== me?.id).length} onPress={() => startCall(false)} />
        )}
        <FlatList
          ref={list}
          data={chat.messages}
          inverted
          keyExtractor={(m) => String(m.id)}
          renderItem={({ item }) => {
            const mine = item.sender_id === me?.id;
            return (
              <MessageBubble
                message={item}
                mine={mine}
                sender={chat.isGroup && !mine ? (item.sender ?? chat.members.get(item.sender_id)) : undefined}
                actions={actionsFor(item)}
                selected={selected === item.id}
                onToggleSelect={() => setSelected((s) => (s === item.id ? null : item.id))}
                onOpenMenu={openMenu}
              />
            );
          }}
          onEndReached={chat.loadOlder}
          onEndReachedThreshold={0.3}
          ListEmptyComponent={<Text style={styles.empty}>Say hi to {title} 👋</Text>}
          contentContainerStyle={{ padding: spacing.lg, flexGrow: 1 }}
        />
        {editing && (
          <EditBar
            icon="edit-2"
            text={`Editing: ${editing.body || 'picture'}`}
            cancelLabel="Cancel editing"
            onCancel={() => setEditing(null)}
          />
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
            if (!editing) return void (await chat.send(body, attachments));
            await chat.edit(editing, body);
            setEditing(null);
          }}
        />
        {menu}
      </View>
    </KeyboardScreen>
  );
}

// Voice and video call buttons, plus the group's details for group chats.
function HeaderButtons({ isGroup, groupId, onCall }: { isGroup: boolean; groupId: string; onCall: (video: boolean) => void }) {
  const { colors } = useTheme();
  const router = useRouter();
  return (
    <View style={{ flexDirection: 'row', gap: spacing.md }}>
      <Pressable onPress={() => onCall(false)} hitSlop={8} accessibilityLabel={isGroup ? 'Group voice call' : 'Voice call'}>
        <Feather name="phone" size={22} color={colors.accent} />
      </Pressable>
      <Pressable onPress={() => onCall(true)} hitSlop={8} accessibilityLabel={isGroup ? 'Group video call' : 'Video call'}>
        <Feather name="video" size={22} color={colors.accent} />
      </Pressable>
      {isGroup && (
        <Pressable onPress={() => router.push(`/group/${groupId}`)} hitSlop={8} accessibilityLabel="Group details">
          <Feather name="info" size={22} color={colors.accent} />
        </Pressable>
      )}
    </View>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    empty: { color: colors.muted, textAlign: 'center', transform: [{ scaleY: -1 }] },
  });
