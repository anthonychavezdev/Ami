import { WS_URL } from './config';
import type { ClientMessage, Event, EventOf, EventType } from './events';

type Handler<T extends EventType> = (e: EventOf<T>) => void;

// Small auto-reconnecting WebSocket client. Events from the server are
// dispatched by their "type" field to the handlers registered with `on`
// (see events.ts for every type and its fields).
class SocketClient {
  private ws: WebSocket | null = null;
  private token: string | null = null;
  private handlers = new Map<string, Set<(e: Event) => void>>();
  private retry = 0;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private closedByUs = false;

  connect(token: string) {
    this.token = token;
    this.closedByUs = false;
    this.open();
  }

  disconnect() {
    this.closedByUs = true;
    this.token = null;
    if (this.timer) clearTimeout(this.timer);
    this.ws?.close();
    this.ws = null;
  }

  get connected() {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  send(msg: ClientMessage): boolean {
    if (!this.connected) return false;
    this.ws!.send(JSON.stringify(msg));
    return true;
  }

  on<T extends EventType>(type: T, handler: Handler<T>): () => void {
    let set = this.handlers.get(type);
    if (!set) this.handlers.set(type, (set = new Set()));
    set.add(handler as (e: Event) => void);
    return () => set!.delete(handler as (e: Event) => void);
  }

  private emit(e: Event) {
    this.handlers.get(e.type)?.forEach((h) => h(e));
  }

  private open() {
    if (!this.token) return;
    const ws = new WebSocket(`${WS_URL}?token=${encodeURIComponent(this.token)}`);
    this.ws = ws;
    ws.onopen = () => {
      this.retry = 0;
      this.emit({ type: 'open' });
    };
    ws.onmessage = (ev) => {
      try {
        this.emit(JSON.parse(String(ev.data)));
      } catch {
        /* ignore malformed frames */
      }
    };
    ws.onerror = () => {};
    ws.onclose = () => {
      if (this.ws === ws) this.ws = null;
      this.emit({ type: 'close' });
      if (this.closedByUs) return;
      const delay = Math.min(30000, 1000 * 2 ** this.retry++);
      this.timer = setTimeout(() => this.open(), delay);
    };
  }
}

export const socket = new SocketClient();
