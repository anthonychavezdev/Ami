import { Feather } from '@expo/vector-icons';
import React from 'react';
import { Platform, Pressable, StyleSheet, Text, View } from 'react-native';

import { ActionLink } from './ActionLink';
import { AttachmentList } from './Attachments';
import { contextMenuProps } from './ContextMenu';
import { DotsButton } from './DotsButton';
import { type MenuItem, deleteItem, editItem, menuItems } from './menu';
import { Avatar } from './ui';
import { clock } from '@/lib/time';
import type { Message, User } from '@/lib/types';
import { useStyles, useTheme } from '@/lib/theme';
import { useHovered } from '@/lib/useHovered';
import { radius, spacing, type ThemeColors } from '@/theme';

// What the viewer may do with a message.
export type MessageActions = { edit?: () => void; remove?: () => void };

// One chat message. Mine sit on the right; in group chats the others' carry
// the author. Hold (native) or hover/right-click (web) reveals the actions.
export function MessageBubble({
  message,
  mine,
  sender,
  actions,
  selected,
  onToggleSelect,
  onOpenMenu,
}: {
  message: Message;
  mine: boolean;
  sender?: User; // shown above the text (group chats, other people's messages)
  actions: MessageActions;
  selected: boolean; // native: the Edit / Delete row is showing
  onToggleSelect: () => void;
  onOpenMenu: (items: MenuItem[], x: number, y: number) => void;
}) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const [hovered, hoverProps] = useHovered(); // web: shows the ⋯ button
  const items = menuItems(editItem(actions.edit), deleteItem(actions.remove));
  const manageable = items.length > 0;
  const hasAttachments = message.attachments?.length > 0;
  const showSender = sender !== undefined;
  const onSurface = mine ? { color: 'rgba(255,255,255,0.75)' } : undefined;

  return (
    <View style={[styles.row, mine && { alignItems: 'flex-end' }]}>
      <View style={[styles.line, mine && { justifyContent: 'flex-end' }]}>
        {showSender && <View style={styles.senderAvatar}>{sender ? <Avatar user={sender} size={28} status={false} /> : null}</View>}
        <Pressable
          {...(manageable ? contextMenuProps((x, y) => onOpenMenu(items, x, y)) : {})}
          {...(manageable ? hoverProps : {})}
          onLongPress={manageable ? onToggleSelect : undefined}
          delayLongPress={400}
          style={[styles.bubble, mine ? styles.mine : styles.theirs, hasAttachments && styles.wide]}
        >
          {manageable && Platform.OS === 'web' && hovered && (
            <DotsButton
              label="Message options"
              style={[styles.dots, mine ? { left: -30 } : { right: -30 }]}
              onOpen={(x, y) => onOpenMenu(items, x, y)}
            />
          )}
          {showSender && <Text style={styles.senderName}>{sender?.display_name ?? 'Former member'}</Text>}
          {message.edited_at ? (
            <View style={styles.edited} accessibilityLabel="edited">
              <Feather name="edit-2" size={10} color={mine ? 'rgba(255,255,255,0.75)' : colors.muted} />
              <Text style={[styles.editedText, onSurface]}>edited</Text>
            </View>
          ) : null}
          {message.body ? <Text style={[styles.text, mine && { color: '#fff' }]}>{message.body}</Text> : null}
          {hasAttachments && <AttachmentList items={message.attachments} style={{ marginTop: message.body ? spacing.sm : 0 }} />}
          <Text style={[styles.time, mine && { color: 'rgba(255,255,255,0.7)' }]}>{clock(message.created_at)}</Text>
        </Pressable>
      </View>
      {selected && (
        <View style={[styles.actions, showSender && { marginLeft: 36 }]}>
          {actions.edit && <ActionLink icon="edit-2" label="Edit message" text="Edit" onPress={actions.edit} />}
          {actions.remove && <ActionLink icon="trash-2" label="Delete message" text="Delete" onPress={actions.remove} />}
        </View>
      )}
    </View>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    row: { marginBottom: spacing.sm, alignItems: 'flex-start' },
    line: { flexDirection: 'row', alignItems: 'flex-end', gap: spacing.sm, maxWidth: '100%' },
    senderAvatar: { width: 28, height: 28, marginBottom: 2 },
    senderName: { fontSize: 12, fontWeight: '700', color: colors.accent, marginBottom: 2 },
    bubble: { maxWidth: '80%', paddingHorizontal: spacing.md, paddingVertical: spacing.sm, borderRadius: radius.lg },
    wide: { width: '80%' },
    mine: { backgroundColor: colors.accent, borderBottomRightRadius: 4 },
    theirs: { backgroundColor: colors.card, borderBottomLeftRadius: 4, borderWidth: 1, borderColor: colors.border },
    dots: { position: 'absolute', top: 0, bottom: 0, width: 26, alignItems: 'center', justifyContent: 'center' },
    text: { fontSize: 16, color: colors.text },
    time: { fontSize: 11, color: colors.muted, marginTop: 2, alignSelf: 'flex-end' },
    edited: { flexDirection: 'row', alignItems: 'center', gap: 3, marginBottom: 2 },
    editedText: { fontSize: 10, color: colors.muted },
    actions: { flexDirection: 'row', gap: spacing.lg, marginTop: 4, paddingHorizontal: spacing.sm },
  });
