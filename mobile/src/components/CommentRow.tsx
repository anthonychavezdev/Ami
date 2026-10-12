import { Feather } from '@expo/vector-icons';
import React, { useState } from 'react';
import { Platform, Pressable, StyleSheet, Text, View } from 'react-native';

import { contextMenuProps, pressPoint, type MenuItem } from './ContextMenu';
import { Avatar } from './ui';
import type { Threaded } from '@/lib/comments';
import { timeAgo } from '@/lib/time';
import { useStyles, useTheme } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

// What the viewer may do with a comment: reply to any live one, edit and delete their own.
export type CommentActions = { reply: () => void; edit?: () => void; remove?: () => void };

export const commentMenuItems = ({ reply, edit, remove }: CommentActions): MenuItem[] => [
  { label: 'Reply', icon: 'corner-down-right', onPress: reply },
  ...(edit ? [{ label: 'Edit', icon: 'edit-2', onPress: edit } as MenuItem] : []),
  ...(remove ? [{ label: 'Delete', icon: 'trash-2', danger: true, onPress: remove } as MenuItem] : []),
];

// One comment in the thread, indented by its depth; deleted ones are placeholders.
export function CommentRow({ comment, actions, onOpenMenu }: { comment: Threaded; actions: CommentActions | null; onOpenMenu: (items: MenuItem[], x: number, y: number) => void }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const [hovered, setHovered] = useState(false); // web: shows the ⋯ button
  const items = actions ? commentMenuItems(actions) : [];
  return (
    <View
      style={[styles.comment, { marginLeft: spacing.lg + Math.min(comment.depth, 6) * 14 }, comment.depth > 0 && styles.nested]}
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
      {...(actions ? contextMenuProps((x, y) => onOpenMenu(items, x, y)) : {})}
    >
      {comment.deleted ? (
        <View style={styles.deletedAvatar}>
          <Feather name="user" size={16} color={colors.muted} />
        </View>
      ) : (
        <Avatar user={comment.author} size={32} />
      )}
      <View style={{ flex: 1 }}>
        <Text style={[styles.meta, { paddingRight: 24 }]}>
          <Text style={[styles.name, comment.deleted && styles.deletedText]}>{comment.author.display_name}</Text> · {timeAgo(comment.created_at)}
          {comment.edited_at ? (
            <Text accessibilityLabel="edited">
              {' · '}
              <Feather name="edit-2" size={10} color={colors.muted} /> edited
            </Text>
          ) : null}
        </Text>
        <Text style={[styles.body, comment.deleted && styles.deletedText]}>{comment.body}</Text>
        {Platform.OS === 'web' && actions && hovered && (
          <Pressable onPress={(e) => onOpenMenu(items, pressPoint(e).x, pressPoint(e).y)} hitSlop={8} style={styles.dots} accessibilityLabel="Comment options">
            <Feather name="more-horizontal" size={18} color={colors.muted} />
          </Pressable>
        )}
        {actions && (
          <View style={styles.actions}>
            <ActionLink icon="corner-down-right" label="Reply" text="Reply" onPress={actions.reply} />
            {actions.edit && <ActionLink icon="edit-2" label="Edit comment" text="Edit" onPress={actions.edit} />}
            {actions.remove && <ActionLink icon="trash-2" label="Delete comment" text="Delete" onPress={actions.remove} />}
          </View>
        )}
      </View>
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
  comment: { flexDirection: 'row', gap: spacing.md, paddingRight: spacing.lg, paddingVertical: spacing.sm },
  nested: { borderLeftWidth: 2, borderLeftColor: colors.border, paddingLeft: spacing.sm },
  name: { fontWeight: '700', color: colors.text },
  meta: { color: colors.muted, fontSize: 13 },
  body: { color: colors.text, fontSize: 15, lineHeight: 20 },
  deletedText: { color: colors.muted, fontStyle: 'italic', fontWeight: '400' },
  deletedAvatar: { width: 32, height: 32, borderRadius: 16, backgroundColor: colors.border, alignItems: 'center', justifyContent: 'center' },
  actions: { flexDirection: 'row', gap: spacing.lg, marginTop: 4 },
  dots: { position: 'absolute', right: 0, top: 0 },
  action: { flexDirection: 'row', alignItems: 'center', gap: 4 },
  actionText: { color: colors.muted, fontSize: 12, fontWeight: '600' },
});
