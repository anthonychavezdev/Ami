import type { CallGroup, Comment, Me, Message, Post, User } from './types';

// Everything that travels over the WebSocket, mirroring server/events.go field
// for field: a change there is a change here. `socket.on` and `socket.send`
// (ws.ts) are typed with these, so a handler only sees the fields its event has.

export type SDP = { type: string; sdp: string };

// ---- server -> client

export type ServerEvent =
  | { type: 'hello'; user: Me; unread: number; online: number[] }
  | { type: 'presence'; user_id: number; online: boolean }
  | { type: 'pong' }
  | { type: 'error'; error: string }
  // Recipients get their unread total with a new message; the sender's other devices get a plain copy.
  | { type: 'message'; message: Message; sender: User; group?: CallGroup; unread?: number }
  | { type: 'message.updated'; message: Message }
  | { type: 'message.deleted'; id: number; sender_id: number; recipient_id: number | null; group_id: number | null; unread: number }
  | { type: 'messages.read'; peer_id?: number; group_id?: number; unread: number }
  | { type: 'group.added'; group: CallGroup; by: User }
  | { type: 'friend.request'; user: User }
  | { type: 'friend.accepted'; user: User }
  | { type: 'post.created'; post: Post }
  | { type: 'post.updated'; post: Post }
  | { type: 'post.deleted'; id: number; group_id: number | null }
  | { type: 'comment.created'; comment: Comment }
  | { type: 'comment.updated'; comment: Comment }
  | { type: 'comment.deleted'; post_id: number; id: number }
  | { type: 'call.ring'; room: string; video: boolean; from_user: User; group?: CallGroup }
  | { type: 'call.joined'; room: string; video: boolean; group?: CallGroup; participants: User[] }
  | { type: 'call.peer_joined'; room: string; user: User }
  | { type: 'call.peer_left'; room: string; user_id: number }
  | { type: 'call.declined'; room: string; user_id: number; reason: string }
  | { type: 'call.ended'; room: string }
  | { type: 'call.unavailable'; room: string; user_id: number }
  // Signaling relayed from another participant.
  | { type: 'call.offer'; room: string; from: number; from_user: User; sdp: SDP }
  | { type: 'call.answer'; room: string; from: number; sdp: SDP }
  | { type: 'call.ice'; room: string; from: number; candidate: unknown }
  | { type: 'call.streams'; room: string; from: number; camera: string | null; screen: string | null };

// The socket reports its own state with these; they never come from the server.
export type SocketEvent = { type: 'open' } | { type: 'close' };

export type Event = ServerEvent | SocketEvent;
export type EventType = Event['type'];
export type EventOf<T extends EventType> = Extract<Event, { type: T }>;

// ---- client -> server

// Who to call: a friend, a group, or the room a call.ring named.
export type CallJoin = { user_id: number; video: boolean } | { group_id: number; video: boolean } | { room: string };

// Signaling to one participant; the manager adds the room and the recipient.
export type CallSignal =
  | { type: 'call.offer'; sdp: SDP }
  | { type: 'call.answer'; sdp: SDP }
  | { type: 'call.ice'; candidate: unknown }
  | { type: 'call.streams'; camera: string | null; screen: string | null };

export type ClientMessage =
  | { type: 'ping' }
  | ({ type: 'call.join' } & CallJoin)
  | { type: 'call.leave'; room: string }
  | { type: 'call.decline'; room: string; reason?: string }
  | (CallSignal & { room: string; to: number });
