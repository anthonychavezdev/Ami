import { Feather } from '@expo/vector-icons';
import { Link, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Platform, Pressable, StyleSheet, Text, View } from 'react-native';

import { AttachmentList } from './Attachments';
import { Composer } from './Composer';
import { contextMenuProps, useContextMenu } from './ContextMenu';
import { DotsButton } from './DotsButton';
import { deleteItem, editItem, menuItems } from './menu';
import { ReactionButton } from './ReactionButton';
import { Avatar } from './ui';
import { ApiError, api } from '@/lib/api';
import { confirmAction } from '@/lib/confirm';
import { useAuth } from '@/lib/auth';
import { PostMarkdown } from '@/lib/markdown';
import { timeAgo } from '@/lib/time';
import type { Post } from '@/lib/types';
import { useStyles, useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { useHovered } from '@/lib/useHovered';
import { radius, spacing, type ThemeColors } from '@/theme';
import { errorMessage } from '@/lib/errors';

type PostActions = { edit?: () => void; remove?: () => void };

// Reactions sorted by count, biggest first.
const sortedReactions = (post: Post) => Object.entries(post.reactions).sort((a, b) => b[1] - a[1]);

export function PostCard({
  post,
  onChange,
  onDelete,
  onComment,
  canDelete,
  showGroup = true,
}: {
  post: Post;
  onChange: (p: Post) => void;
  onDelete?: (id: number) => void;
  onComment?: () => void; // on the post screen: focus the comment box instead of opening the post
  canDelete?: boolean;
  showGroup?: boolean;
}) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const { user } = useAuth();
  const router = useRouter();
  const [editing, setEditing] = useState(false);
  const [hovered, hoverProps] = useHovered(); // web: shows the ⋯ button
  const { open, menu } = useContextMenu();
  const react = useReact(post, onChange, onDelete);

  const mine = post.author.id === user?.id;
  const actions: PostActions = {
    edit: mine ? () => setEditing(true) : undefined,
    remove: mine || canDelete ? () => confirmAction('Delete post?', undefined, 'Delete', () => deletePost(post.id, onDelete)) : undefined,
  };
  const items = menuItems(editItem(actions.edit), deleteItem(actions.remove));
  const openMenu = (x: number, y: number) => open(items, x, y);

  return (
    <View style={styles.card} {...hoverProps} {...(items.length ? contextMenuProps(openMenu) : {})}>
      <View style={styles.header}>
        <AuthorLine post={post} showGroup={showGroup} />
        <View style={styles.headerActions}>
          {Platform.OS === 'web' ? (
            items.length > 0 && !editing && hovered && <DotsButton label="Post options" size={20} onOpen={openMenu} />
          ) : (
            <>
              {actions.edit && !editing && (
                <Pressable onPress={actions.edit} hitSlop={10} accessibilityLabel="Edit post">
                  <Feather name="edit-2" size={17} color={colors.muted} />
                </Pressable>
              )}
              {actions.remove && (
                <Pressable onPress={actions.remove} hitSlop={10} accessibilityLabel="Delete post">
                  <Feather name="trash-2" size={17} color={colors.muted} />
                </Pressable>
              )}
            </>
          )}
        </View>
      </View>

      {editing ? (
        <Composer
          rich
          autoFocus
          initialText={post.body}
          initialAttachments={post.attachments}
          placeholder="Edit your post"
          buttonTitle="Save"
          onCancel={() => setEditing(false)}
          onSubmit={async (body, attachments) => {
            onChange(await api.updatePost(post.id, body, attachments));
            setEditing(false);
          }}
        />
      ) : (
        <>
          {post.body ? (
            <View style={styles.body}>
              <PostMarkdown>{post.body}</PostMarkdown>
            </View>
          ) : null}
          {post.attachments?.length > 0 && <AttachmentList items={post.attachments} />}
        </>
      )}

      <ReactionSummary post={post} />

      <View style={styles.footer}>
        <ReactionButton current={post.my_reaction} onReact={react} />
        <Pressable onPress={onComment ?? (() => router.push(`/post/${post.id}`))} style={styles.commentBtn}>
          <Feather name="message-circle" size={16} color={colors.muted} />
          <Text style={styles.commentText}>{post.comment_count > 0 ? `${post.comment_count} ` : ''}Comment</Text>
        </Pressable>
      </View>
      {menu}
    </View>
  );
}

// Avatar, name, handle, age, group and the "edited" mark.
function AuthorLine({ post, showGroup }: { post: Post; showGroup: boolean }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  return (
    <Link href={`/user/${post.author.id}`} asChild>
      <Pressable style={styles.author}>
        <Avatar user={post.author} />
        <View style={{ flex: 1 }}>
          <Text style={styles.name}>{post.author.display_name}</Text>
          <Text style={styles.meta}>
            @{post.author.username} · {timeAgo(post.created_at)}
            {showGroup && post.group_name ? ` · in ${post.group_name}` : ''}
            {post.edited_at ? (
              <Text accessibilityLabel="edited">
                {' · '}
                <Feather name="edit-2" size={11} color={colors.muted} /> edited
              </Text>
            ) : null}
          </Text>
        </View>
      </Pressable>
    </Link>
  );
}

function ReactionSummary({ post }: { post: Post }) {
  const styles = useStyles(makeStyles);
  const reactions = sortedReactions(post);
  if (reactions.length === 0) return null;
  return (
    <View style={styles.reactions}>
      {reactions.map(([emoji, count]) => (
        <View key={emoji} style={[styles.reactionPill, emoji === post.my_reaction && styles.reactionMine]}>
          <Text style={{ fontSize: 14 }}>
            {emoji} {count}
          </Text>
        </View>
      ))}
    </View>
  );
}

// Sets or clears the viewer's reaction, one request at a time; a post that
// vanished meanwhile is reported and dropped.
function useReact(post: Post, onChange: (p: Post) => void, onDelete?: (id: number) => void) {
  const [busy, setBusy] = useState(false);
  return async (emoji: string | null) => {
    if (busy) return;
    setBusy(true);
    try {
      onChange(emoji ? await api.react(post.id, emoji) : await api.unreact(post.id));
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        toast.show('That post was deleted');
        onDelete?.(post.id);
      } else {
        toast.error(errorMessage(e));
      }
    } finally {
      setBusy(false);
    }
  };
}

async function deletePost(id: number, onDelete?: (id: number) => void) {
  try {
    await api.deletePost(id);
    onDelete?.(id);
  } catch (e) {
    toast.error(errorMessage(e));
  }
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    card: {
      backgroundColor: colors.card,
      borderRadius: radius.md,
      padding: spacing.lg,
      marginHorizontal: spacing.lg,
      marginBottom: spacing.md,
      borderWidth: 1,
      borderColor: colors.border,
    },
    header: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
    headerActions: { flexDirection: 'row', alignItems: 'center', gap: spacing.md },
    author: { flex: 1, flexDirection: 'row', alignItems: 'center', gap: spacing.md },
    name: { fontWeight: '700', color: colors.text, fontSize: 15 },
    meta: { color: colors.muted, fontSize: 13 },
    body: { marginTop: spacing.md },
    reactions: { flexDirection: 'row', flexWrap: 'wrap', gap: 6, marginTop: spacing.md },
    reactionPill: {
      backgroundColor: colors.bg,
      borderRadius: radius.pill,
      paddingHorizontal: 8,
      paddingVertical: 3,
      borderWidth: 1,
      borderColor: colors.border,
    },
    reactionMine: { borderColor: colors.accent, backgroundColor: colors.accentSoft },
    footer: {
      flexDirection: 'row',
      alignItems: 'center',
      justifyContent: 'space-between',
      marginTop: spacing.md,
      borderTopWidth: 1,
      borderTopColor: colors.border,
      paddingTop: spacing.sm,
    },
    commentBtn: { flexDirection: 'row', alignItems: 'center', gap: 6, paddingVertical: 6, paddingHorizontal: 12 },
    commentText: { color: colors.muted, fontWeight: '600' },
  });
