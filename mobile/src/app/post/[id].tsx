import { Feather } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FlatList, KeyboardAvoidingView, Platform, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';

import { Composer } from '@/components/Composer';
import { PostCard } from '@/components/PostCard';
import { Avatar, Empty, ErrorBanner, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { confirmAction } from '@/lib/confirm';
import { timeAgo } from '@/lib/time';
import { toast } from '@/lib/toast';
import type { Comment, Post } from '@/lib/types';
import { socket } from '@/lib/ws';
import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

type Threaded = Comment & { depth: number };

// Replies go under their parent, oldest first, like Reddit.
function thread(comments: Comment[]): Threaded[] {
  const children = new Map<number | null, Comment[]>();
  for (const c of comments) {
    const key = c.parent_id ?? null;
    if (!children.has(key)) children.set(key, []);
    children.get(key)!.push(c);
  }
  const out: Threaded[] = [];
  const walk = (parent: number | null, depth: number) => {
    for (const c of children.get(parent) ?? []) {
      out.push({ ...c, depth });
      walk(c.id, depth + 1);
    }
  };
  walk(null, 0);
  return out;
}

export default function PostScreen() {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const { id } = useLocalSearchParams<{ id: string }>();
  const postId = Number(id);
  const router = useRouter();
  const { user: me } = useAuth();
  const commentInput = useRef<TextInput>(null);
  const [post, setPost] = useState<Post | null>(null);
  const [comments, setComments] = useState<Comment[]>([]);
  const [replyTo, setReplyTo] = useState<Comment | null>(null);
  const [editing, setEditing] = useState<Comment | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const threaded = useMemo(() => thread(comments), [comments]);

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

  // Live: comments come, change and go; reaction/count changes; deletion of this post.
  useEffect(() => {
    const refetch = () => void api.comments(postId).then(setComments).catch(() => {});
    const offs = [
      socket.on('comment.created', (e) => {
        const c: Comment = e.comment;
        if (c.post_id === postId) setComments((prev) => (prev.some((x) => x.id === c.id) ? prev : [...prev, c]));
      }),
      socket.on('comment.updated', (e) => {
        const c: Comment = e.comment;
        if (c.post_id === postId) setComments((prev) => prev.map((x) => (x.id === c.id ? c : x)));
      }),
      socket.on('comment.deleted', (e) => {
        if (e.post_id === postId) refetch(); // the server decides what stays as [deleted]
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

  const startReply = (c: Comment) => {
    setEditing(null);
    setReplyTo(c);
  };
  const startEdit = (c: Comment) => {
    setReplyTo(null);
    setEditing(c);
  };
  const remove = (c: Comment) =>
    confirmAction('Delete comment?', c.body, 'Delete', async () => {
      try {
        await api.deleteComment(c.id);
        setComments(await api.comments(postId));
        if (editing?.id === c.id) setEditing(null);
        if (replyTo?.id === c.id) setReplyTo(null);
      } catch (e) {
        toast.error(e instanceof Error ? e.message : String(e));
      }
    });

  if (loading) return <Loading />;
  if (!post) return <Empty text={error ?? 'Post not found'} />;

  const mode = editing ? 'edit' : replyTo ? 'reply' : 'new';
  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1 }} keyboardVerticalOffset={90}>
      <FlatList
        data={threaded}
        keyExtractor={(c) => String(c.id)}
        ListHeaderComponent={
          <View style={{ paddingTop: spacing.lg }}>
            <PostCard post={post} onChange={setPost} onDelete={() => router.back()} onComment={() => commentInput.current?.focus()} />
            <ErrorBanner error={error} />
            <Text style={styles.heading}>Comments</Text>
          </View>
        }
        renderItem={({ item }) => {
          const mine = !item.deleted && item.author.id === me?.id;
          return (
            <View style={[styles.comment, { marginLeft: spacing.lg + Math.min(item.depth, 6) * 14 }, item.depth > 0 && styles.nested]}>
              {item.deleted ? (
                <View style={styles.deletedAvatar}>
                  <Feather name="user" size={16} color={colors.muted} />
                </View>
              ) : (
                <Avatar user={item.author} size={32} />
              )}
              <View style={{ flex: 1 }}>
                <Text style={styles.meta}>
                  <Text style={[styles.name, item.deleted && styles.deletedText]}>{item.author.display_name}</Text> · {timeAgo(item.created_at)}
                  {item.edited_at ? (
                    <Text accessibilityLabel="edited">
                      {' · '}
                      <Feather name="edit-2" size={10} color={colors.muted} /> edited
                    </Text>
                  ) : null}
                </Text>
                <Text style={[styles.body, item.deleted && styles.deletedText]}>{item.body}</Text>
                {!item.deleted && (
                  <View style={styles.actions}>
                    <Pressable onPress={() => startReply(item)} hitSlop={6} style={styles.action} accessibilityLabel="Reply">
                      <Feather name="corner-down-right" size={13} color={colors.muted} />
                      <Text style={styles.actionText}>Reply</Text>
                    </Pressable>
                    {mine && (
                      <Pressable onPress={() => startEdit(item)} hitSlop={6} style={styles.action} accessibilityLabel="Edit comment">
                        <Feather name="edit-2" size={13} color={colors.muted} />
                        <Text style={styles.actionText}>Edit</Text>
                      </Pressable>
                    )}
                    {mine && (
                      <Pressable onPress={() => remove(item)} hitSlop={6} style={styles.action} accessibilityLabel="Delete comment">
                        <Feather name="trash-2" size={13} color={colors.muted} />
                        <Text style={styles.actionText}>Delete</Text>
                      </Pressable>
                    )}
                  </View>
                )}
              </View>
            </View>
          );
        }}
        ListEmptyComponent={<Text style={styles.empty}>Be the first to comment.</Text>}
        contentContainerStyle={{ paddingBottom: spacing.lg }}
      />
      {mode !== 'new' && (
        <View style={styles.bar}>
          <Feather name={mode === 'edit' ? 'edit-2' : 'corner-down-right'} size={14} color={colors.accent} />
          <Text style={styles.barText} numberOfLines={1}>
            {mode === 'edit' ? `Editing: ${editing!.body}` : `Replying to ${replyTo!.author.display_name}`}
          </Text>
          <Pressable
            onPress={() => {
              setEditing(null);
              setReplyTo(null);
            }}
            hitSlop={8}
            accessibilityLabel="Cancel"
          >
            <Feather name="x" size={18} color={colors.muted} />
          </Pressable>
        </View>
      )}
      <Composer
        key={mode === 'edit' ? `edit-${editing!.id}` : mode === 'reply' ? `reply-${replyTo!.id}` : 'new'}
        inputRef={commentInput}
        placeholder={mode === 'reply' ? 'Write a reply…' : 'Write a comment…'}
        buttonTitle={mode === 'edit' ? 'Save' : 'Send'}
        initialText={editing?.body}
        autoFocus={mode !== 'new'}
        multiline={false}
        compact
        onSubmit={async (body) => {
          if (editing) {
            const c = await api.updateComment(editing.id, body);
            setComments((prev) => prev.map((x) => (x.id === c.id ? c : x)));
            setEditing(null);
            return;
          }
          const c = await api.addComment(postId, body, replyTo?.id);
          setComments((prev) => (prev.some((x) => x.id === c.id) ? prev : [...prev, c]));
          setPost((p) => (p ? { ...p, comment_count: p.comment_count + 1 } : p));
          setReplyTo(null);
        }}
      />
    </KeyboardAvoidingView>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  heading: { fontWeight: '700', color: colors.text, fontSize: 16, paddingHorizontal: spacing.lg, marginTop: spacing.sm, marginBottom: spacing.sm },
  comment: { flexDirection: 'row', gap: spacing.md, paddingRight: spacing.lg, paddingVertical: spacing.sm },
  nested: { borderLeftWidth: 2, borderLeftColor: colors.border, paddingLeft: spacing.sm },
  name: { fontWeight: '700', color: colors.text },
  meta: { color: colors.muted, fontSize: 13 },
  body: { color: colors.text, fontSize: 15, lineHeight: 20 },
  deletedText: { color: colors.muted, fontStyle: 'italic', fontWeight: '400' },
  deletedAvatar: { width: 32, height: 32, borderRadius: 16, backgroundColor: colors.border, alignItems: 'center', justifyContent: 'center' },
  actions: { flexDirection: 'row', gap: spacing.lg, marginTop: 4 },
  action: { flexDirection: 'row', alignItems: 'center', gap: 4 },
  actionText: { color: colors.muted, fontSize: 12, fontWeight: '600' },
  bar: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, paddingHorizontal: spacing.lg, paddingVertical: spacing.sm, backgroundColor: colors.card, borderTopWidth: 1, borderTopColor: colors.border, borderRadius: radius.sm },
  barText: { flex: 1, color: colors.text, fontSize: 13 },
  empty: { color: colors.muted, paddingHorizontal: spacing.lg },
});
