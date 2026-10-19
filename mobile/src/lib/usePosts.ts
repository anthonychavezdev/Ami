import { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react';

import { errorMessage } from './errors';
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
  const [loadedFor, setLoadedFor] = useState<unknown>(null); // the fetchPage whose first page is in
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cursor = useRef<number | undefined>(undefined); // id of the oldest post loaded
  const done = useRef(false); // the last page was empty
  const busy = useRef(false);

  const load = useCallback(
    async (reset: boolean) => {
      if (busy.current || (!reset && done.current)) return;
      busy.current = true;
      try {
        const page = await fetchPage(reset ? undefined : cursor.current);
        done.current = page.length === 0;
        if (reset || page.length) cursor.current = page[page.length - 1]?.id;
        setPosts((prev) => (reset ? page : withPage(prev, page)));
        setLoadedFor(() => fetchPage); // a function given straight to the setter would be run as an updater
        setError(null);
      } catch (e) {
        setError(errorMessage(e));
      } finally {
        busy.current = false;
        setRefreshing(false);
      }
    },
    [fetchPage],
  );

  useEffect(() => {
    done.current = false;
    void load(true);
  }, [load]);

  const prepend = useCallback((p: Post) => setPosts((prev) => withPost(prev, p)), []);
  const onCreated = useEffectEvent((p: Post) => accept(p) && prepend(p)); // always the latest accept

  useEffect(() => {
    const offs = [
      socket.on('post.created', (e) => onCreated(e.post)),
      socket.on('post.updated', (e) => setPosts((prev) => replacePost(prev, e.post))),
      socket.on('post.deleted', (e) => setPosts((prev) => withoutPost(prev, e.id))),
    ];
    return () => offs.forEach((off) => off());
  }, [prepend]);

  return {
    posts,
    loading: loadedFor !== fetchPage,
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
