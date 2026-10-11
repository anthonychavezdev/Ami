import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, KeyboardAvoidingView, Platform, StyleSheet, Text, View } from 'react-native';

import { Composer } from '@/components/Composer';
import { PostCard } from '@/components/PostCard';
import { Avatar, Empty, ErrorBanner, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { timeAgo } from '@/lib/time';
import type { Comment, Post } from '@/lib/types';
import { colors, spacing } from '@/theme';

export default function PostScreen() {
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

  if (loading) return <Loading />;
  if (!post) return <Empty text={error ?? 'Post not found'} />;

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1 }} keyboardVerticalOffset={90}>
      <FlatList
        data={comments}
        keyExtractor={(c) => String(c.id)}
        ListHeaderComponent={
          <View style={{ paddingTop: spacing.lg }}>
            <PostCard post={post} onDelete={() => router.back()} />
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
          setComments((prev) => [...prev, c]);
          setPost((p) => (p ? { ...p, comment_count: p.comment_count + 1 } : p));
        }}
      />
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  heading: { fontWeight: '700', color: colors.text, fontSize: 16, paddingHorizontal: spacing.lg, marginTop: spacing.sm, marginBottom: spacing.sm },
  comment: { flexDirection: 'row', gap: spacing.md, paddingHorizontal: spacing.lg, paddingVertical: spacing.sm },
  name: { fontWeight: '700', color: colors.text },
  meta: { color: colors.muted, fontSize: 13 },
  body: { color: colors.text, fontSize: 15, lineHeight: 20 },
  empty: { color: colors.muted, paddingHorizontal: spacing.lg },
});
