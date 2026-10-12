// Browser build: react-native-web has no Alert, so use the browser's dialog.
export function confirmAction(title: string, message: string | undefined, action: string, onConfirm: () => void) {
  if (window.confirm(`${title}${message ? `\n${message}` : ''}\n\n[OK] ${action}`)) onConfirm();
}
