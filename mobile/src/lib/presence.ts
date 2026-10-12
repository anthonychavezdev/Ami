import { useSyncExternalStore } from 'react';

import { socket } from './ws';

// Who is online right now. Seeded by the `online` flag on user objects as they
// are rendered, kept current by presence events and the hello list.
const known = new Map<number, boolean>();
const listeners = new Set<() => void>();
let version = 0;

function changed() {
  version++;
  listeners.forEach((l) => l());
}

socket.on('presence', (e) => {
  known.set(e.user_id, !!e.online);
  changed();
});
socket.on('hello', (e) => {
  const online = new Set<number>(e.online ?? []);
  for (const id of known.keys()) known.set(id, online.has(id));
  for (const id of online) known.set(id, true);
  changed();
});

const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => void listeners.delete(l);
};
const snapshot = () => version;

export function useOnline(userId: number | undefined, fallback?: boolean): boolean {
  useSyncExternalStore(subscribe, snapshot, snapshot);
  if (!userId) return false;
  return known.get(userId) ?? !!fallback;
}
