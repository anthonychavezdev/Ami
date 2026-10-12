import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, KeyboardAvoidingView, Platform, StyleSheet, Text, View } from 'react-native';

import { Composer } from '@/components/Composer';
import { PostCard } from '@/components/PostCard';
import { Avatar, Empty, ErrorBanner, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { timeAgo } from '@/lib/time';
import { toast } from '@/lib/toast';
import type { Comment, Post } from '@/lib/types';
import { socket } from '@/lib/ws';
import { useStyles } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

export default function PostScreen() {
  const styles = useStyles(makeStyles);
  const { id } = useLocalSearchParams<{ id: string }>();
  const postId = Number(id);
  const router = useRouter();
  const [post, setPost] = useState<Post | null>(null);
  const [comments, setComments] = useState<Comment[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      const [p, c] = await Promise.all([api.post(postId), api.comments(postId)]);
      setPost(p);
      setComments(c);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [postId]);

  useEffect(() => {
    void load();
  }, [load]);

  // Live: new comments, reaction/count changes, and deletion of this post.
  useEffect(() => {
    const offs = [
      socket.on('comment.created', (e) => {
        const c: Comment = e.comment;
        if (c.post_id === postId) setComments((prev) => (prev.some((x) => x.id === c.id) ? prev : [...prev, c]));
      }),
      socket.on('post.updated', (e) => {
        if (e.post.id === postId) setPost(e.post);
      }),
      socket.on('post.deleted', (e) => {
        if (e.id === postId) {
          toast.show('This post was deleted');
          router.back();
        }
      }),
    ];
    return () => offs.forEach((off) => off());
  }, [postId, router]);

  if (loading) return <Loading />;
  if (!post) return <Empty text={error ?? 'Post not found'} />;

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1 }} keyboardVerticalOffset={90}>
      <FlatList
        data={comments}
        keyExtractor={(c) => String(c.id)}
        ListHeaderComponent={
          <View style={{ paddingTop: spacing.lg }}>
            <PostCard post={post} onChange={setPost} onDelete={() => router.back()} />
            <ErrorBanner error={error} />
            <Text style={styles.heading}>Comments</Text>
          </View>
        }
        renderItem={({ item }) => (
          <View style={styles.comment}>
            <Avatar user={item.author} size={32} />
            <View style={{ flex: 1 }}>
              <Text style={styles.meta}>
                <Text style={styles.name}>{item.author.display_name}</Text> · {timeAgo(item.created_at)}
              </Text>
              <Text style={styles.body}>{item.body}</Text>
            </View>
          </View>
        )}
        ListEmptyComponent={<Text style={styles.empty}>Be the first to comment.</Text>}
        contentContainerStyle={{ paddingBottom: spacing.lg }}
      />
      <Composer
        placeholder="Write a comment…"
        buttonTitle="Send"
        multiline={false}
        compact
        onSubmit={async (body) => {
          const c = await api.addComment(postId, body);
          setComments((prev) => (prev.some((x) => x.id === c.id) ? prev : [...prev, c]));
          setPost((p) => (p ? { ...p, comment_count: p.comment_count + 1 } : p));
        }}
      />
    </KeyboardAvoidingView>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  heading: { fontWeight: '700', color: colors.text, fontSize: 16, paddingHorizontal: spacing.lg, marginTop: spacing.sm, marginBottom: spacing.sm },
  comment: { flexDirection: 'row', gap: spacing.md, paddingHorizontal: spacing.lg, paddingVertical: spacing.sm },
  name: { fontWeight: '700', color: colors.text },
  meta: { color: colors.muted, fontSize: 13 },
  body: { color: colors.text, fontSize: 15, lineHeight: 20 },
  empty: { color: colors.muted, paddingHorizontal: spacing.lg },
});
