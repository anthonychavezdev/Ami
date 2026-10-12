// Browser notifications through the Notification API; the WebSocket keeps
// delivering while the tab is open, even in the background.
let onTap: ((data: Record<string, any>) => void) | null = null;
let count = 0;

export async function setupNotifications() {
  if (typeof Notification !== 'undefined' && Notification.permission === 'default') await Notification.requestPermission().catch(() => {});
}

export async function teardownNotifications() {
  setBadge(0);
}

export async function notify(title: string, body: string, data: Record<string, unknown>) {
  if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return;
  const n = new Notification(title, {
    body,
    tag: data.group_chat ? `group-${String(data.group_chat)}` : `chat-${String(data.chat)}`,
    icon: '/favicon.ico',
  });
  n.onclick = () => {
    window.focus();
    onTap?.(data);
    n.close();
  };
}

export function onNotificationTap(handler: (data: Record<string, any>) => void): () => void {
  onTap = handler;
  return () => {
    onTap = null;
  };
}

// "(3) Ami" in the tab title; the router rewrites the title on navigation, so
// keep re-applying the prefix.
export function setBadge(n: number) {
  count = n;
  applyTitle();
}

function applyTitle() {
  const base = document.title.replace(/^\(\d+\) /, '');
  const title = count > 0 ? `(${count}) ${base}` : base;
  if (document.title !== title) document.title = title;
}

if (typeof document !== 'undefined') {
  const el = document.querySelector('title');
  if (el) new MutationObserver(applyTitle).observe(el, { childList: true, characterData: true, subtree: true });
}

export function isHidden() {
  return document.hidden;
}
