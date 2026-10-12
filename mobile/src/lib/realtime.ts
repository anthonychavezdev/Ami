import { callManager } from './call';
import { listenPresence } from './presence';
import { listenUnread } from './unread';

let started = false;

// Registers every WebSocket listener the app has (unread badge, presence,
// calls) once, before the socket first connects. Screens add their own
// listeners while they are mounted.
export function startRealtime() {
  if (started) return;
  started = true;
  listenUnread();
  listenPresence();
  callManager.listen();
}
