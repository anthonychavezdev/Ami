import { useCallback, useEffect, useRef, useState } from 'react';

import type { Post } from './types';

// Cursor-paginated post list shared by the feed, profiles and groups.
export function usePosts(fetchPage: (before?: number) => Promise<Post[]>) {
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const done = useRef(false);
  const busy = useRef(false);

  const load = useCallback(
    async (reset: boolean) => {
      if (busy.current) return;
      busy.current = true;
      try {
        const before = reset || posts.length === 0 ? undefined : posts[posts.length - 1].id;
        if (!reset && done.current) return;
        const page = await fetchPage(before);
        done.current = page.length === 0;
        setPosts((prev) => (reset ? page : [...prev, ...page.filter((p) => !prev.some((x) => x.id === p.id))]));
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
    update: (p: Post) => setPosts((prev) => prev.map((x) => (x.id === p.id ? p : x))),
    remove: (id: number) => setPosts((prev) => prev.filter((x) => x.id !== id)),
    prepend: (p: Post) => setPosts((prev) => [p, ...prev]),
  };
}
