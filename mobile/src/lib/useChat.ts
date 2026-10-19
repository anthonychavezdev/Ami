import { useCallback, useEffect, useMemo, useState } from 'react';
import { AppState } from 'react-native';

import { api } from './api';
import { errorMessage } from './errors';
import { belongsTo, groupRoomKey, mergeLatest, prependMessage, removeMessage, replaceMessage } from './messages';
import type { Attachment, ChatTarget, Group, GroupMember, Message, User } from './types';
import { unread } from './unread';
import { socket } from './ws';

// Everything a chat screen needs: who the chat is with, its messages (newest
// first, kept live over the WebSocket), the read marker, and for groups who is
// in the group call right now.
export function useChat(kind: ChatTarget['kind'], id: number) {
  const chat = useMemo<ChatTarget>(() => ({ kind, id }), [kind, id]);
  const isGroup = kind === 'group';
  const [other, setOther] = useState<User | null>(null);
  const [group, setGroup] = useState<Group | null>(null);
  const [members, setMembers] = useState<ReadonlyMap<number, GroupMember>>(new Map());
  const [inCall, setInCall] = useState<number[]>([]); // members in the group call
  const [messages, setMessages] = useState<Message[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const who = isGroup
      ? api.group(id).then((g) => {
          setGroup(g.group);
          setMembers(new Map(g.members.map((m) => [m.id, m])));
          setInCall(g.call);
        })
      : api.user(id).then(setOther);
    Promise.all([who, api.messages(chat)])
      .then(([, list]) => {
        setMessages(list.slice().reverse());
        void api.markRead(chat);
      })
      .catch((e) => setError(errorMessage(e)));
  }, [chat, id, isGroup]);

  // While this chat is open, new messages in it are read, not notified.
  useEffect(() => {
    unread.setActiveChat(chat);
    const sub = AppState.addEventListener('change', (s) => s === 'active' && void api.markRead(chat));
    return () => {
      unread.setActiveChat(null);
      sub.remove();
    };
  }, [chat]);

  useEffect(() => {
    const inThisChat = belongsTo(chat);
    const offs = [
      socket.on('message', (e) => inThisChat(e.message) && setMessages((list) => list && prependMessage(list, e.message))),
      socket.on('message.updated', (e) => inThisChat(e.message) && setMessages((list) => list && replaceMessage(list, e.message))),
      socket.on('message.deleted', (e) => inThisChat(e) && setMessages((list) => list && removeMessage(list, e.id))),
      // Reconnected: whatever was said or changed meanwhile was missed, so fetch the newest page again.
      socket.on('open', () => void api.messages(chat).then((latest) => setMessages((list) => list && mergeLatest(list, latest)))),
    ];
    return () => offs.forEach((off) => off());
  }, [chat]);

  useEffect(() => {
    if (!isGroup) return;
    const room = groupRoomKey(id);
    const offs = [
      socket.on('call.ring', (e) => e.room === room && setInCall([e.from_user.id])),
      socket.on('call.peer_joined', (e) => e.room === room && setInCall((ids) => (ids.includes(e.user.id) ? ids : [...ids, e.user.id]))),
      socket.on('call.peer_left', (e) => e.room === room && setInCall((ids) => ids.filter((x) => x !== e.user_id))),
      socket.on('call.ended', (e) => e.room === room && setInCall([])),
    ];
    return () => offs.forEach((off) => off());
  }, [isGroup, id]);

  const loadOlder = useCallback(async () => {
    const oldest = messages?.[messages.length - 1];
    if (!oldest) return;
    const older = await api.messages(chat, oldest.id);
    if (older.length) setMessages((list) => [...(list ?? []), ...older.slice().reverse()]);
  }, [messages, chat]);

  // Sending, editing and deleting show their result at once from the server's
  // reply, like posts and comments do; the socket's echo of the same change is
  // then a no-op. Otherwise a dropped socket would hide your own edits.
  const send = useCallback(
    async (body: string, attachments: Attachment[]) => {
      const m = await api.sendMessage(chat, body, attachments);
      setMessages((list) => list && prependMessage(list, m));
    },
    [chat],
  );
  const edit = useCallback(
    async (m: Message, body: string) => {
      const updated = await api.updateMessage(chat, m.id, body);
      setMessages((list) => list && replaceMessage(list, updated));
    },
    [chat],
  );
  const remove = useCallback(
    async (m: Message) => {
      await api.deleteMessage(chat, m.id);
      setMessages((list) => list && removeMessage(list, m.id));
    },
    [chat],
  );

  return { chat, isGroup, other, group, members, inCall, messages, error, loadOlder, send, edit, remove };
}
