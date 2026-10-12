import type { Message } from './types';

// Text of a message, or what kind of attachment it carries.
export const messagePreview = (m: Message) => m.body || (m.attachments?.[0]?.type === 'video' ? 'Video' : m.attachments?.[0]?.type === 'link' ? 'Link' : 'Picture');
