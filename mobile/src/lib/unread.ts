import { useSyncExternalStore } from 'react';

import { api } from './api';
import { chatKey, chatOf, messagePreview } from './messages';
import { isHidden, notify } from './notifications';
import { setBadge } from './notifications';
import type { ChatTarget, Message, User } from './types';
import { socket } from './ws';

// Unread message count for the Messages tab badge. The server is the source
// of truth (hello / message / messages.read events carry the total); this
// store also decides when an incoming message deserves an OS notification.
let count = 0;
let me: number | null = null;
let activeChat: string | null = null; // chat screen currently open (see chatKey)
const listeners = new Set<() => void>();

function set(n: number) {
  if (n === count) return;
  count = n;
  setBadge(n);
  listeners.forEach((l) => l());
}

export const unread = {
  subscribe(l: () => void) {
    listeners.add(l);
    return () => void listeners.delete(l);
  },
  get: () => count,
  setUser(id: number | null) {
    me = id;
    if (id === null) set(0);
  },
  setActiveChat(chat: ChatTarget | null) {
    activeChat = chat ? chatKey(chat) : null;
  },
};

socket.on('hello', (e) => set(e.unread ?? 0));
socket.on('messages.read', (e) => set(e.unread ?? 0));
socket.on('message.deleted', (e) => set(e.unread ?? 0));
socket.on('message', (e) => {
  const m: Message = e.message;
  const sender: User = e.sender;
  if (me === null || m.sender_id === me) return; // our own message, echoed to this device
  const chat = chatOf(m, me);
  if (chatKey(chat) === activeChat && !isHidden()) {
    void api.markRead(chat); // being read right now
    return;
  }
  if (typeof e.unread === 'number') set(e.unread);
  const title = m.group_id ? `${e.group?.name ?? 'Group'} · ${sender.display_name}` : sender.display_name;
  void notify(title, messagePreview(m), m.group_id ? { group_chat: m.group_id } : { chat: m.sender_id });
});

export function useUnread() {
  return useSyncExternalStore(unread.subscribe, unread.get, unread.get);
}
