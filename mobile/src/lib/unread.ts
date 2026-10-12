import { useSyncExternalStore } from 'react';

import { api } from './api';
import { messagePreview } from './messages';
import { isHidden, notify } from './notifications';
import { setBadge } from './notifications';
import type { Message, User } from './types';
import { socket } from './ws';

// Unread message count for the Messages tab badge. The server is the source
// of truth (hello / message / messages.read events carry the total); this
// store also decides when an incoming message deserves an OS notification.
let count = 0;
let me: number | null = null;
let activeChat: number | null = null; // chat screen currently open
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
  setActiveChat(id: number | null) {
    activeChat = id;
  },
};

socket.on('hello', (e) => set(e.unread ?? 0));
socket.on('messages.read', (e) => set(e.unread ?? 0));
socket.on('message.deleted', (e) => set(e.unread ?? 0));
socket.on('message', (e) => {
  const m: Message = e.message;
  const sender: User = e.sender;
  if (m.recipient_id !== me) return; // our own message, echoed to this device
  if (m.sender_id === activeChat && !isHidden()) {
    void api.markRead(m.sender_id); // being read right now
    return;
  }
  if (typeof e.unread === 'number') set(e.unread);
  void notify(sender.display_name, messagePreview(m), { chat: m.sender_id });
});

export function useUnread() {
  return useSyncExternalStore(unread.subscribe, unread.get, unread.get);
}
