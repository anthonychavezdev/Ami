import type { ChatTarget, Message } from './types';

// Text of a message, or what kind of attachment it carries.
export const messagePreview = (m: Message) => m.body || (m.attachments?.[0]?.type === 'video' ? 'Video' : m.attachments?.[0]?.type === 'link' ? 'Link' : 'Picture');

// The chat a message belongs to, seen from `me`.
export const chatOf = (m: Message, me: number): ChatTarget =>
  m.group_id ? { kind: 'group', id: m.group_id } : { kind: 'user', id: m.sender_id === me ? (m.recipient_id ?? m.sender_id) : m.sender_id };

export const chatKey = (chat: ChatTarget) => `${chat.kind === 'group' ? 'g' : 'u'}:${chat.id}`;

// Route of a chat screen.
export const chatRoute = (chat: ChatTarget) => (chat.kind === 'group' ? `/chat/group/${chat.id}` : `/chat/${chat.id}`);
