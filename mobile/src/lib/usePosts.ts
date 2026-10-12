import { useCallback, useEffect, useRef, useState } from 'react';

import type { Post } from './types';
import { socket } from './ws';

// List helpers; each returns a new list.
const withPost = (list: Post[], p: Post) => (list.some((x) => x.id === p.id) ? list : [p, ...list]);
const withPage = (list: Post[], page: Post[]) => [...list, ...page.filter((p) => !list.some((x) => x.id === p.id))];
const replacePost = (list: Post[], p: Post) => list.map((x) => (x.id === p.id ? p : x));
const withoutPost = (list: Post[], id: number) => list.filter((x) => x.id !== id);

// Cursor-paginated post list shared by the feed, profiles and groups. It stays
// live: posts created, changed or deleted arrive over the WebSocket and are
// filtered through `accept` (does this post belong in this list?).
export function usePosts(fetchPage: (before?: number) => Promise<Post[]>, accept: (p: Post) => boolean = () => true) {
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const done = useRef(false); // the last page was empty
  const busy = useRef(false);
  const acceptRef = useRef(accept);
  acceptRef.current = accept;

  const load = useCallback(
    async (reset: boolean) => {
      if (busy.current || (!reset && done.current)) return;
      busy.current = true;
      try {
        const before = reset || posts.length === 0 ? undefined : posts[posts.length - 1].id;
        const page = await fetchPage(before);
        done.current = page.length === 0;
        setPosts((prev) => (reset ? page : withPage(prev, page)));
        setError(null);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        busy.current = false;
        setLoading(false);
        setRefreshing(false);
      }
    },
    [fetchPage, posts],
  );

  useEffect(() => {
    done.current = false;
    setLoading(true);
    void load(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchPage]);

  const prepend = useCallback((p: Post) => setPosts((prev) => withPost(prev, p)), []);

  useEffect(() => {
    const offs = [
      socket.on('post.created', (e) => acceptRef.current(e.post) && prepend(e.post)),
      socket.on('post.updated', (e) => setPosts((prev) => replacePost(prev, e.post))),
      socket.on('post.deleted', (e) => setPosts((prev) => withoutPost(prev, e.id))),
    ];
    return () => offs.forEach((off) => off());
  }, [prepend]);

  return {
    posts,
    loading,
    refreshing,
    error,
    refresh: () => {
      setRefreshing(true);
      done.current = false;
      void load(true);
    },
    loadMore: () => void load(false),
    update: (p: Post) => setPosts((prev) => replacePost(prev, p)),
    remove: (id: number) => setPosts((prev) => withoutPost(prev, id)),
    prepend,
  };
}
