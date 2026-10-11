import { Link, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Alert, Pressable, StyleSheet, Text, View } from 'react-native';

import { ReactionButton } from './ReactionButton';
import { Avatar } from './ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { timeAgo } from '@/lib/time';
import type { Post } from '@/lib/types';
import { colors, radius, spacing } from '@/theme';

export function PostCard({
  post,
  onChange,
  onDelete,
  canDelete,
  showGroup = true,
}: {
  post: Post;
  onChange: (p: Post) => void;
  onDelete?: (id: number) => void;
  canDelete?: boolean;
  showGroup?: boolean;
}) {
  const { user } = useAuth();
  const router = useRouter();
  const [busy, setBusy] = useState(false);

  const react = async (emoji: string | null) => {
    if (busy) return;
    setBusy(true);
    try {
      onChange(emoji ? await api.react(post.id, emoji) : await api.unreact(post.id));
    } catch (e) {
      Alert.alert('Reaction failed', e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = () =>
    Alert.alert('Delete post?', undefined, [
      { text: 'Cancel', style: 'cancel' },
      {
        text: 'Delete',
        style: 'destructive',
        onPress: async () => {
          try {
            await api.deletePost(post.id);
            onDelete?.(post.id);
          } catch (e) {
            Alert.alert('Delete failed', e instanceof Error ? e.message : String(e));
          }
        },
      },
    ]);

  const reactions = Object.entries(post.reactions).sort((a, b) => b[1] - a[1]);
  const total = reactions.reduce((n, [, c]) => n + c, 0);
  const mayDelete = canDelete || post.author.id === user?.id;

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
              </Text>
            </View>
          </Pressable>
        </Link>
        {mayDelete && (
          <Pressable onPress={remove} hitSlop={10}>
            <Text style={styles.meta}>✕</Text>
          </Pressable>
        )}
      </View>

      <Text style={styles.body}>{post.body}</Text>

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
        <Pressable onPress={() => router.push(`/post/${post.id}`)} style={styles.commentBtn}>
          <Text style={styles.commentText}>💬 {post.comment_count > 0 ? `${post.comment_count} ` : ''}Comment</Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { backgroundColor: colors.card, borderRadius: radius.md, padding: spacing.lg, marginHorizontal: spacing.lg, marginBottom: spacing.md, borderWidth: 1, borderColor: colors.border },
  header: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  author: { flex: 1, flexDirection: 'row', alignItems: 'center', gap: spacing.md },
  name: { fontWeight: '700', color: colors.text, fontSize: 15 },
  meta: { color: colors.muted, fontSize: 13 },
  body: { color: colors.text, fontSize: 16, lineHeight: 22, marginTop: spacing.md },
  reactions: { flexDirection: 'row', flexWrap: 'wrap', gap: 6, marginTop: spacing.md },
  reactionPill: { backgroundColor: colors.bg, borderRadius: radius.pill, paddingHorizontal: 8, paddingVertical: 3, borderWidth: 1, borderColor: colors.border },
  reactionMine: { borderColor: colors.accent, backgroundColor: colors.accentSoft },
  footer: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: spacing.md, borderTopWidth: 1, borderTopColor: colors.border, paddingTop: spacing.sm },
  commentBtn: { paddingVertical: 6, paddingHorizontal: 12 },
  commentText: { color: colors.muted, fontWeight: '600' },
});
