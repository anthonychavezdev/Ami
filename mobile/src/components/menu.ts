import type { Feather } from '@expo/vector-icons';
import type React from 'react';

// An entry of the web context menu (ContextMenu.web.tsx); native shows the
// same actions as inline controls instead.
export type MenuItem = { label: string; icon: React.ComponentProps<typeof Feather>['name']; danger?: boolean; onPress: () => void };

// The usual entries. An action the viewer does not have (undefined) leaves no entry.
export const menuItems = (...items: (MenuItem | undefined)[]): MenuItem[] => items.filter((i): i is MenuItem => !!i);
export const replyItem = (reply?: () => void) => reply && { label: 'Reply', icon: 'corner-down-right' as const, onPress: reply };
export const editItem = (edit?: () => void) => edit && { label: 'Edit', icon: 'edit-2' as const, onPress: edit };
export const deleteItem = (remove?: () => void) => remove && { label: 'Delete', icon: 'trash-2' as const, danger: true, onPress: remove };
