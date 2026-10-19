import type { MediaStream } from './webrtc';
import type { CallGroup, User } from './types';

export type CallStatus = 'idle' | 'incoming' | 'outgoing' | 'connecting' | 'active';

// Someone else in the call and what they send us.
export interface Participant {
  user: User;
  streams: MediaStream[]; // their camera/microphone stream and, while they share, their screen
  screenId: string | null; // id of the stream that is their screen
  connected: boolean;
}

export interface CallState {
  status: CallStatus;
  room: string | null; // the server's room key, once known
  group: CallGroup | null; // a group call
  peer: User | null; // one-to-one: the other person; while ringing: whoever is calling
  participants: Participant[];
  video: boolean; // placed as a video call
  localStream: MediaStream | null;
  localScreen: MediaStream | null;
  muted: boolean;
  cameraOn: boolean;
  sharing: boolean;
  error: string | null;
  version: number; // bumped whenever tracks change so views re-render
}

export const IDLE: CallState = {
  status: 'idle',
  room: null,
  group: null,
  peer: null,
  participants: [],
  video: false,
  localStream: null,
  localScreen: null,
  muted: false,
  cameraOn: false,
  sharing: false,
  error: null,
  version: 0,
};

// Pure transitions: each returns a new state and leaves the old one untouched.

export const inCall = (status: CallStatus) => status === 'outgoing' || status === 'connecting' || status === 'active';

export const participantOf = (state: CallState, id: string) => state.participants.find((p) => p.user.id === id);

export function withParticipant(state: CallState, user: User): CallState {
  if (participantOf(state, user.id)) return state;
  return { ...state, participants: [...state.participants, { user, streams: [], screenId: null, connected: false }] };
}

export function withoutParticipant(state: CallState, id: string): CallState {
  return { ...state, participants: state.participants.filter((p) => p.user.id !== id) };
}

export function updateParticipant(state: CallState, id: string, change: (p: Participant) => Participant): CallState {
  return { ...state, participants: state.participants.map((p) => (p.user.id === id ? change(p) : p)) };
}

// A stream the participant started sending (ignored if already known).
export const withStream =
  (stream: MediaStream) =>
  (p: Participant): Participant =>
    p.streams.some((s) => s.id === stream.id) ? p : { ...p, streams: [...p.streams, stream] };

// What the participant says their screen stream is; a dropped share forgets that stream.
export const withScreen =
  (screenId: string | null) =>
  (p: Participant): Participant => ({
    ...p,
    screenId,
    streams: screenId ? p.streams : p.streams.filter((s) => s.id !== p.screenId),
  });

export const connected = (p: Participant): Participant => ({ ...p, connected: true });

// The call screen's view of the state.
export const cameraOf = (p: Participant) => p.streams.find((s) => s.id !== p.screenId && s.getVideoTracks().length > 0) ?? null;

export const sharerOf = (state: CallState) =>
  state.participants.find((p) => p.screenId && p.streams.some((s) => s.id === p.screenId)) ?? null;
