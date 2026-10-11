import { Link } from 'expo-router';
import React from 'react';
import { Alert, Pressable, StyleSheet, Text, View } from 'react-native';

import { Avatar } from './ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { timeAgo } from '@/lib/time';
import type { Post } from '@/lib/types';
import { colors, radius, spacing } from '@/theme';

export function PostCard({ post, onDelete }: { post: Post; onDelete?: (id: number) => void }) {
  const { user } = useAuth();

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

  const mayDelete = post.author.id === user?.id;

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
});
