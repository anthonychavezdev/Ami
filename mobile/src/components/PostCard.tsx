import { Feather } from '@expo/vector-icons';
import { Link, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { AttachmentList } from './Attachments';
import { Composer } from './Composer';
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
import { radius, spacing, type ThemeColors } from '@/theme';

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
  const [busy, setBusy] = useState(false);

  const react = async (emoji: string | null) => {
    if (busy) return;
    setBusy(true);
    try {
      onChange(emoji ? await api.react(post.id, emoji) : await api.unreact(post.id));
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) {
        toast.show('That post was deleted');
        onDelete?.(post.id);
      } else {
        toast.error(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setBusy(false);
    }
  };

  const remove = () =>
    confirmAction('Delete post?', undefined, 'Delete', async () => {
      try {
        await api.deletePost(post.id);
        onDelete?.(post.id);
      } catch (e) {
        toast.error(e instanceof Error ? e.message : String(e));
      }
    });

  const reactions = Object.entries(post.reactions).sort((a, b) => b[1] - a[1]);
  const total = reactions.reduce((n, [, c]) => n + c, 0);
  const mayDelete = canDelete || post.author.id === user?.id;
  const mine = post.author.id === user?.id;
  const [editing, setEditing] = useState(false);

  return (
    <View style={styles.card}>
      <View style={styles.header}>
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
        <View style={styles.headerActions}>
          {mine && !editing && (
            <Pressable onPress={() => setEditing(true)} hitSlop={10} accessibilityLabel="Edit post">
              <Feather name="edit-2" size={17} color={colors.muted} />
            </Pressable>
          )}
          {mayDelete && (
            <Pressable onPress={remove} hitSlop={10} accessibilityLabel="Delete post">
              <Feather name="trash-2" size={17} color={colors.muted} />
            </Pressable>
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

      {total > 0 && (
        <View style={styles.reactions}>
          {reactions.map(([emoji, count]) => (
            <View key={emoji} style={[styles.reactionPill, emoji === post.my_reaction && styles.reactionMine]}>
              <Text style={{ fontSize: 14 }}>
                {emoji} {count}
              </Text>
            </View>
          ))}
        </View>
      )}

      <View style={styles.footer}>
        <ReactionButton current={post.my_reaction} onReact={react} />
        <Pressable onPress={onComment ?? (() => router.push(`/post/${post.id}`))} style={styles.commentBtn}>
          <Feather name="message-circle" size={16} color={colors.muted} />
          <Text style={styles.commentText}>{post.comment_count > 0 ? `${post.comment_count} ` : ''}Comment</Text>
        </Pressable>
      </View>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  card: { backgroundColor: colors.card, borderRadius: radius.md, padding: spacing.lg, marginHorizontal: spacing.lg, marginBottom: spacing.md, borderWidth: 1, borderColor: colors.border },
  header: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  headerActions: { flexDirection: 'row', alignItems: 'center', gap: spacing.md },
  author: { flex: 1, flexDirection: 'row', alignItems: 'center', gap: spacing.md },
  name: { fontWeight: '700', color: colors.text, fontSize: 15 },
  meta: { color: colors.muted, fontSize: 13 },
  body: { marginTop: spacing.md },
  reactions: { flexDirection: 'row', flexWrap: 'wrap', gap: 6, marginTop: spacing.md },
  reactionPill: { backgroundColor: colors.bg, borderRadius: radius.pill, paddingHorizontal: 8, paddingVertical: 3, borderWidth: 1, borderColor: colors.border },
  reactionMine: { borderColor: colors.accent, backgroundColor: colors.accentSoft },
  footer: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: spacing.md, borderTopWidth: 1, borderTopColor: colors.border, paddingTop: spacing.sm },
  commentBtn: { flexDirection: 'row', alignItems: 'center', gap: 6, paddingVertical: 6, paddingHorizontal: 12 },
  commentText: { color: colors.muted, fontWeight: '600' },
});
