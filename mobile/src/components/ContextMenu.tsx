import type React from 'react';

import type { MenuItem } from './menu';

// Native: touch has hold-to-select and inline controls, so there is no
// context menu. ContextMenu.web.tsx is the browser version.

export function useContextMenu(): { open: (items: MenuItem[], x: number, y: number) => void; close: () => void; menu: React.ReactNode } {
  return { open: () => {}, close: () => {}, menu: null };
}

// Props that open the menu on right-click (web only).
export function contextMenuProps(_open: (x: number, y: number) => void): Record<string, unknown> {
  return {};
}

// Pointer position of a press event, for placing the menu at the ⋯ button.
export function pressPoint(_e: unknown): { x: number; y: number } {
  return { x: 0, y: 0 };
}
