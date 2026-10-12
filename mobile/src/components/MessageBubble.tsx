import { Feather } from '@expo/vector-icons';
import React, { useState } from 'react';
import { Platform, Pressable, StyleSheet, Text, View } from 'react-native';

import { AttachmentList } from './Attachments';
import { contextMenuProps, pressPoint, type MenuItem } from './ContextMenu';
import { Avatar } from './ui';
import { clock } from '@/lib/time';
import type { Message, User } from '@/lib/types';
import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

// What the viewer may do with a message.
export type MessageActions = { edit?: () => void; remove?: () => void };

export const messageMenuItems = ({ edit, remove }: MessageActions): MenuItem[] => [
  ...(edit ? [{ label: 'Edit', icon: 'edit-2', onPress: edit } as MenuItem] : []),
  ...(remove ? [{ label: 'Delete', icon: 'trash-2', danger: true, onPress: remove } as MenuItem] : []),
];

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
  const [hovered, setHovered] = useState(false); // web: shows the ⋯ button
  const items = messageMenuItems(actions);
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
          onLongPress={manageable ? onToggleSelect : undefined}
          delayLongPress={400}
          onHoverIn={manageable ? () => setHovered(true) : undefined}
          onHoverOut={manageable ? () => setHovered(false) : undefined}
          style={[styles.bubble, mine ? styles.mine : styles.theirs, hasAttachments && styles.wide]}
        >
          {manageable && Platform.OS === 'web' && hovered && (
            <Pressable onPress={(e) => onOpenMenu(items, pressPoint(e).x, pressPoint(e).y)} hitSlop={6} style={[styles.dots, mine ? { left: -30 } : { right: -30 }]} accessibilityLabel="Message options">
              <Feather name="more-horizontal" size={18} color={colors.muted} />
            </Pressable>
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

function ActionLink({ icon, label, text, onPress }: { icon: React.ComponentProps<typeof Feather>['name']; label: string; text: string; onPress: () => void }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  return (
    <Pressable onPress={onPress} hitSlop={6} style={styles.action} accessibilityLabel={label}>
      <Feather name={icon} size={13} color={colors.muted} />
      <Text style={styles.actionText}>{text}</Text>
    </Pressable>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
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
  action: { flexDirection: 'row', alignItems: 'center', gap: 4 },
  actionText: { color: colors.muted, fontSize: 12, fontWeight: '600' },
});
