import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useRef, useState } from 'react';
import { FlatList, KeyboardAvoidingView, Platform, StyleSheet, Text, TextInput, View } from 'react-native';

import { CommentRow, type CommentActions } from '@/components/CommentRow';
import { Composer } from '@/components/Composer';
import { useContextMenu } from '@/components/ContextMenu';
import { EditBar } from '@/components/EditBar';
import { PostCard } from '@/components/PostCard';
import { Empty, ErrorBanner, Loading } from '@/components/ui';
import { useAuth } from '@/lib/auth';
import { confirmAction } from '@/lib/confirm';
import { toast } from '@/lib/toast';
import type { Comment } from '@/lib/types';
import { useComments } from '@/lib/useComments';
import { useStyles } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';
import { errorMessage } from '@/lib/errors';

// The composer is writing a new comment, a reply, or an edit.
type Mode = { kind: 'new' } | { kind: 'reply'; to: Comment } | { kind: 'edit'; comment: Comment };

export default function PostScreen() {
  const styles = useStyles(makeStyles);
  const { id } = useLocalSearchParams<{ id: string }>();
  const postId = Number(id);
  const router = useRouter();
  const { user: me } = useAuth();
  const { post, setPost, threaded, error, loading, add, edit, remove } = useComments(postId);
  const commentInput = useRef<TextInput>(null);
  const [mode, setMode] = useState<Mode>({ kind: 'new' });
  const { open: openMenu, menu } = useContextMenu();

  const confirmRemove = (c: Comment) =>
    confirmAction('Delete comment?', c.body, 'Delete', async () => {
      try {
        await remove(c);
        if (mode.kind !== 'new' && ('to' in mode ? mode.to : mode.comment).id === c.id) setMode({ kind: 'new' });
      } catch (e) {
        toast.error(errorMessage(e));
      }
    });

  const actionsFor = (c: Comment): CommentActions | null => {
    if (c.deleted) return null;
    const mine = c.author.id === me?.id;
    return {
      reply: () => setMode({ kind: 'reply', to: c }),
      edit: mine ? () => setMode({ kind: 'edit', comment: c }) : undefined,
      remove: mine ? () => confirmRemove(c) : undefined,
    };
  };

  const submit = async (body: string) => {
    if (mode.kind === 'edit') await edit(mode.comment, body);
    else await add(body, mode.kind === 'reply' ? mode.to.id : undefined);
    setMode({ kind: 'new' });
  };

  if (loading) return <Loading />;
  if (!post) return <Empty text={error ?? 'Post not found'} />;

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
        renderItem={({ item }) => <CommentRow comment={item} actions={actionsFor(item)} onOpenMenu={openMenu} />}
        ListEmptyComponent={<Text style={styles.empty}>Be the first to comment.</Text>}
        contentContainerStyle={{ paddingBottom: spacing.lg }}
      />
      {mode.kind === 'edit' && <EditBar icon="edit-2" text={`Editing: ${mode.comment.body}`} onCancel={() => setMode({ kind: 'new' })} />}
      {mode.kind === 'reply' && (
        <EditBar icon="corner-down-right" text={`Replying to ${mode.to.author.display_name}`} onCancel={() => setMode({ kind: 'new' })} />
      )}
      <Composer
        key={mode.kind === 'edit' ? `edit-${mode.comment.id}` : mode.kind === 'reply' ? `reply-${mode.to.id}` : 'new'}
        inputRef={commentInput}
        placeholder={mode.kind === 'reply' ? 'Write a reply…' : 'Write a comment…'}
        buttonTitle={mode.kind === 'edit' ? 'Save' : 'Send'}
        initialText={mode.kind === 'edit' ? mode.comment.body : undefined}
        autoFocus={mode.kind !== 'new'}
        multiline={false}
        compact
        onSubmit={submit}
      />
      {menu}
    </KeyboardAvoidingView>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    heading: {
      fontWeight: '700',
      color: colors.text,
      fontSize: 16,
      paddingHorizontal: spacing.lg,
      marginTop: spacing.sm,
      marginBottom: spacing.sm,
    },
    empty: { color: colors.muted, paddingHorizontal: spacing.lg },
  });
