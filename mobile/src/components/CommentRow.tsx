import { Feather } from '@expo/vector-icons';
import React from 'react';
import { Platform, StyleSheet, Text, View } from 'react-native';

import { ActionLink } from './ActionLink';
import { contextMenuProps } from './ContextMenu';
import { DotsButton } from './DotsButton';
import { type MenuItem, deleteItem, editItem, menuItems, replyItem } from './menu';
import { ReactionButton } from './ReactionButton';
import { ReactionSummary } from './ReactionSummary';
import { Avatar } from './ui';
import { api } from '@/lib/api';
import type { Threaded } from '@/lib/comments';
import { timeAgo } from '@/lib/time';
import { useStyles, useTheme } from '@/lib/theme';
import { useHovered } from '@/lib/useHovered';
import { spacing, type ThemeColors } from '@/theme';

// What the viewer may do with a comment: react to and reply to any live one, edit and delete their own.
export type CommentActions = { react: (emoji: string | null) => void; reply: () => void; edit?: () => void; remove?: () => void };

// One comment in the thread, indented by its depth; deleted ones are placeholders.
export function CommentRow({
  comment,
  actions,
  onOpenMenu,
}: {
  comment: Threaded;
  actions: CommentActions | null;
  onOpenMenu: (items: MenuItem[], x: number, y: number) => void;
}) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const [hovered, hoverProps] = useHovered(); // web: shows the ⋯ button
  const items = actions ? menuItems(replyItem(actions.reply), editItem(actions.edit), deleteItem(actions.remove)) : [];
  return (
    <View
      style={[styles.comment, { marginLeft: spacing.lg + Math.min(comment.depth, 6) * 14 }, comment.depth > 0 && styles.nested]}
      {...hoverProps}
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
          <Text style={[styles.name, comment.deleted && styles.deletedText]}>{comment.author.display_name}</Text> ·{' '}
          {timeAgo(comment.created_at)}
          {comment.edited_at ? (
            <Text accessibilityLabel="edited">
              {' · '}
              <Feather name="edit-2" size={10} color={colors.muted} /> edited
            </Text>
          ) : null}
        </Text>
        <Text style={[styles.body, comment.deleted && styles.deletedText]}>{comment.body}</Text>
        {!comment.deleted && (
          <ReactionSummary
            compact
            id={comment.id}
            reactions={comment.reactions}
            myReaction={comment.my_reaction}
            load={api.commentReactions}
          />
        )}
        {Platform.OS === 'web' && actions && hovered && (
          <DotsButton label="Comment options" style={styles.dots} onOpen={(x, y) => onOpenMenu(items, x, y)} />
        )}
        {actions && (
          <View style={styles.actions}>
            <ReactionButton compact current={comment.my_reaction} onReact={actions.react} />
            <ActionLink icon="corner-down-right" label="Reply" text="Reply" onPress={actions.reply} />
            {actions.edit && <ActionLink icon="edit-2" label="Edit comment" text="Edit" onPress={actions.edit} />}
            {actions.remove && <ActionLink icon="trash-2" label="Delete comment" text="Delete" onPress={actions.remove} />}
          </View>
        )}
      </View>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    comment: { flexDirection: 'row', gap: spacing.md, paddingRight: spacing.lg, paddingVertical: spacing.sm },
    nested: { borderLeftWidth: 2, borderLeftColor: colors.border, paddingLeft: spacing.sm },
    name: { fontWeight: '700', color: colors.text },
    meta: { color: colors.muted, fontSize: 13 },
    body: { color: colors.text, fontSize: 15, lineHeight: 20 },
    deletedText: { color: colors.muted, fontStyle: 'italic', fontWeight: '400' },
    deletedAvatar: {
      width: 32,
      height: 32,
      borderRadius: 16,
      backgroundColor: colors.border,
      alignItems: 'center',
      justifyContent: 'center',
    },
    actions: { flexDirection: 'row', gap: spacing.lg, marginTop: 4 },
    dots: { position: 'absolute', right: 0, top: 0 },
  });
