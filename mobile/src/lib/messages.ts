import type { ChatTarget, Message } from './types';

// Text of a message, or what kind of attachment it carries.
export const messagePreview = (m: Message) =>
  m.body || (m.attachments?.[0]?.type === 'video' ? 'Video' : m.attachments?.[0]?.type === 'link' ? 'Link' : 'Picture');

// The chat a message belongs to, seen from `me`.
export const chatOf = (m: Message, me: number): ChatTarget =>
  m.group_id ? { kind: 'group', id: m.group_id } : { kind: 'user', id: m.sender_id === me ? (m.recipient_id ?? m.sender_id) : m.sender_id };

export const chatKey = (chat: ChatTarget) => `${chat.kind === 'group' ? 'g' : 'u'}:${chat.id}`;

// The chat a key names (the two-pane Messages view keeps the open chat in the URL), or null.
export function parseChatKey(key: string | undefined): ChatTarget | null {
  const m = key?.match(/^([gu]):(\d+)$/);
  return m ? { kind: m[1] === 'g' ? 'group' : 'user', id: Number(m[2]) } : null;
}

// The server's room key of a group's call (see server/calls.go).
export const groupRoomKey = (groupId: number) => `g:${groupId}`;

// Route of a chat screen.
export const chatRoute = (chat: ChatTarget) => (chat.kind === 'group' ? `/chat/group/${chat.id}` : `/chat/${chat.id}`);

// Whether a message (or a deletion event naming the same fields) is part of a chat.
export const belongsTo = (chat: ChatTarget) => (m: Pick<Message, 'group_id' | 'sender_id' | 'recipient_id'>) =>
  chat.kind === 'group' ? m.group_id === chat.id : m.group_id == null && (m.sender_id === chat.id || m.recipient_id === chat.id);

// List helpers for a chat shown newest first; each returns a new list.
export const prependMessage = (list: Message[], m: Message) => (list.some((x) => x.id === m.id) ? list : [m, ...list]);
export const replaceMessage = (list: Message[], m: Message) => list.map((x) => (x.id === m.id ? m : x));
export const removeMessage = (list: Message[], id: number) => list.filter((x) => x.id !== id);

// Folds the newest page from the server (oldest first) into the list: new
// messages go on top, known ones take the fresh copy, older pages stay.
export function mergeLatest(list: Message[], latest: Message[]): Message[] {
  const fresh = new Map(latest.map((m) => [m.id, m]));
  const unseen = latest.filter((m) => !list.some((x) => x.id === m.id)).reverse();
  return [...unseen, ...list.map((m) => fresh.get(m.id) ?? m)];
}

// The second line of a Messages tab row: who wrote last and what, or how
// many members a silent group has.
export function conversationSubtitle(
  c: { last_message: Message | null; group?: { member_count: number } },
  me: number | undefined,
  when: (iso: string) => string,
): string {
  const last = c.last_message;
  if (!last) return c.group ? `${c.group.member_count} members · no messages yet` : '';
  const who = last.sender_id === me ? 'You' : c.group ? (last.sender?.display_name ?? 'Someone') : null;
  return `${who ? `${who}: ` : ''}${messagePreview(last)} · ${when(last.created_at)}`;
}
