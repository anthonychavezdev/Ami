import { api } from './api';
import { chatKey, chatOf, messagePreview } from './messages';
import { isHidden, notify, setBadge } from './notifications';
import { createStore } from './store';
import type { ChatTarget, Message, User } from './types';
import { socket } from './ws';

// Unread message count for the Messages tab badge. The server is the source
// of truth (hello / message / messages.read events carry the total); this
// store also decides when an incoming message deserves an OS notification.
const count = createStore(0);
let me: number | null = null;
let activeChat: string | null = null; // chat screen currently open (see chatKey)

count.subscribe(() => setBadge(count.get()));

export const unread = {
  setUser(id: number | null) {
    me = id;
    if (id === null) count.set(0);
  },
  setActiveChat(chat: ChatTarget | null) {
    activeChat = chat ? chatKey(chat) : null;
  },
};

export const useUnread = count.use;

// What to do with a message that just arrived.
type Reaction = 'ignore' | 'read' | 'notify';

function reactionTo(m: Message, viewer: number | null, open: string | null, hidden: boolean): Reaction {
  if (viewer === null || m.sender_id === viewer) return 'ignore'; // our own message, echoed to this device
  if (chatKey(chatOf(m, viewer)) === open && !hidden) return 'read'; // being read right now
  return 'notify';
}

function notificationTitle(m: Message, sender: User, groupName?: string) {
  return m.group_id ? `${groupName ?? 'Group'} · ${sender.display_name}` : sender.display_name;
}

socket.on('hello', (e) => count.set(e.unread ?? 0));
socket.on('messages.read', (e) => count.set(e.unread ?? 0));
socket.on('message.deleted', (e) => count.set(e.unread ?? 0));
socket.on('message', (e) => {
  const m: Message = e.message;
  switch (reactionTo(m, me, activeChat, isHidden())) {
    case 'read':
      void api.markRead(chatOf(m, me!));
      return;
    case 'notify':
      if (typeof e.unread === 'number') count.set(e.unread);
      void notify(notificationTitle(m, e.sender, e.group?.name), messagePreview(m), m.group_id ? { group_chat: m.group_id } : { chat: m.sender_id });
      return;
    default:
      return;
  }
});
