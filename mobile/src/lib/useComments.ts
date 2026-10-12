import { useRouter } from 'expo-router';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { api } from './api';
import { addComment, replaceComment, thread } from './comments';
import { toast } from './toast';
import type { Comment, Post } from './types';
import { socket } from './ws';
import { errorMessage } from './errors';

// A post and its comments, kept live: comments come, change and go, the
// post's counts and reactions update, and a deleted post sends you back.
export function useComments(postId: number) {
  const router = useRouter();
  const [post, setPost] = useState<Post | null>(null);
  const [comments, setComments] = useState<Comment[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const threaded = useMemo(() => thread(comments), [comments]);

  useEffect(() => {
    Promise.all([api.post(postId), api.comments(postId)])
      .then(([p, c]) => {
        setPost(p);
        setComments(c);
      })
      .catch((e) => setError(errorMessage(e)))
      .finally(() => setLoading(false));
  }, [postId]);

  useEffect(() => {
    const refetch = () =>
      void api
        .comments(postId)
        .then(setComments)
        .catch(() => {});
    const offs = [
      socket.on('comment.created', (e) => e.comment.post_id === postId && setComments((list) => addComment(list, e.comment))),
      socket.on('comment.updated', (e) => e.comment.post_id === postId && setComments((list) => replaceComment(list, e.comment))),
      socket.on('comment.deleted', (e) => e.post_id === postId && refetch()), // the server decides what stays as [deleted]
      socket.on('post.updated', (e) => e.post.id === postId && setPost(e.post)),
      socket.on('post.deleted', (e) => {
        if (e.id !== postId) return;
        toast.show('This post was deleted');
        router.back();
      }),
    ];
    return () => offs.forEach((off) => off());
  }, [postId, router]);

  const add = useCallback(
    async (body: string, parentId?: number) => {
      const c = await api.addComment(postId, body, parentId);
      setComments((list) => addComment(list, c));
      setPost(await api.post(postId)); // the server counts; a local +1 could double up with its post.updated event
    },
    [postId],
  );
  const edit = useCallback(async (c: Comment, body: string) => {
    const updated = await api.updateComment(c.id, body);
    setComments((list) => replaceComment(list, updated));
  }, []);
  const remove = useCallback(
    async (c: Comment) => {
      await api.deleteComment(c.id);
      setComments(await api.comments(postId));
    },
    [postId],
  );

  return { post, setPost, threaded, error, loading, add, edit, remove };
}
