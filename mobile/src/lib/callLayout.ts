import { type CallState, cameraOf, sharerOf } from './callState';
import type { User } from './types';
import type { MediaStream } from './webrtc';

// How the call screen shows the people in a call. Pure: derived from the call
// state and the viewer, so the screen only renders.

export type Tile = { key: string; user: User; stream: MediaStream | null; mirror?: boolean; label: string };

export type Layout =
  | { kind: 'portrait' } // ringing, or alone in the room: one big avatar
  | { kind: 'one'; tile: Tile } // one-to-one: them full screen, us in a corner
  | { kind: 'stage'; stage: MediaStream; local: boolean; tiles: Tile[] } // a shared screen with everyone in a strip
  | { kind: 'grid'; tiles: Tile[] };

export const localVideoOf = (state: CallState) => (state.cameraOn && state.localStream?.getVideoTracks().length ? state.localStream : null);

export function callLayout(state: CallState, me: User): Layout {
  const { participants } = state;
  if (state.status === 'incoming' || participants.length === 0) return { kind: 'portrait' };
  const tiles = participants.map<Tile>((p) => ({ key: String(p.user.id), user: p.user, stream: cameraOf(p), label: p.user.display_name }));
  const localTile: Tile = { key: 'me', user: me, stream: localVideoOf(state), mirror: true, label: 'You' };
  // A shared screen takes the stage: someone else's first, otherwise our own.
  const sharer = sharerOf(state);
  const stage = sharer ? sharer.streams.find((s) => s.id === sharer.screenId)! : state.localScreen;
  if (stage) return { kind: 'stage', stage, local: !sharer, tiles: [...tiles, localTile] };
  if (participants.length === 1) return { kind: 'one', tile: tiles[0] };
  return { kind: 'grid', tiles: [...tiles, localTile] };
}

export function callSubtitle(state: CallState): string {
  const sharer = sharerOf(state);
  const kind = state.video ? 'video' : 'voice';
  switch (state.status) {
    case 'incoming':
      return state.group ? `${state.peer?.display_name ?? 'Someone'} is calling · ${kind}` : `Incoming ${kind} call`;
    case 'outgoing':
      return 'Calling…';
    case 'connecting':
      return 'Connecting…';
    case 'active':
      if (state.sharing) return 'You are sharing your screen';
      if (sharer) return `${sharer.user.display_name} is sharing their screen`;
      if (state.group)
        return state.participants.length === 0 ? 'Waiting for others to join…' : `${state.participants.length + 1} in the call`;
      return state.video || state.cameraOn ? 'Video call' : 'Voice call';
    default:
      return '';
  }
}

// Columns of the grid: 2 up to four people, 3 beyond.
export const gridColumns = (n: number) => (n <= 1 ? 1 : n <= 4 ? 2 : 3);
