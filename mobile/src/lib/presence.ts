import { createStore } from './store';
import { socket } from './ws';

// Who is online right now. Seeded by the `online` flag on user objects as they
// are rendered, kept current by presence events and the hello list.
const online = createStore<ReadonlyMap<number, boolean>>(new Map());

// Follows presence events (called once by startRealtime).
export function listenPresence() {
  socket.on('presence', (e) => online.set((known) => new Map(known).set(e.user_id, e.online)));
  socket.on('hello', (e) => online.set((known) => withOnlineList(known, e.online)));
}

// Everyone we knew about is offline unless listed; everyone listed is online.
function withOnlineList(known: ReadonlyMap<number, boolean>, ids: number[]): ReadonlyMap<number, boolean> {
  const listed = new Set(ids);
  const next = new Map([...known.keys()].map((id) => [id, listed.has(id)] as const));
  ids.forEach((id) => next.set(id, true));
  return next;
}

export function useOnline(userId: number | undefined, fallback?: boolean): boolean {
  const known = online.use();
  if (!userId) return false;
  return known.get(userId) ?? !!fallback;
}
