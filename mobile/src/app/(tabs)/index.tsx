import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback } from 'react';
import { FlatList, RefreshControl } from 'react-native';

import { Composer } from '@/components/Composer';
import { PostCard } from '@/components/PostCard';
import { PostView } from '@/components/PostView';
import { Empty, ErrorBanner, Loading, Panes } from '@/components/ui';
import { api } from '@/lib/api';
import { useWide } from '@/lib/layout';
import { usePosts } from '@/lib/usePosts';
import { useTheme } from '@/lib/theme';
import { spacing } from '@/theme';

// Your posts and those of your friends and groups. On a wide window the open
// post sits beside the feed with its comments, named by ?post= so a reload or
// a link (/post/... redirects here) lands on it.
export default function FeedScreen() {
  const { colors } = useTheme();
  const router = useRouter();
  const wide = useWide();
  const { post: postParam } = useLocalSearchParams<{ post?: string }>();
  const openId = wide && postParam ? postParam : null;
  const fetchPage = useCallback((before?: string) => api.feed(before), []);
  const { posts, loading, refreshing, error, refresh, loadMore, update, remove, prepend } = usePosts(fetchPage);

  const list = (
    <FlatList
      data={posts}
      keyExtractor={(p) => String(p.id)}
      renderItem={({ item }) => (
        <PostCard
          post={item}
          onChange={update}
          onDelete={remove}
          selected={item.id === openId}
          onComment={wide ? () => router.setParams({ post: item.id }) : undefined}
        />
      )}
      ListHeaderComponent={
        <>
          <Composer
            rich
            placeholder="What's on your mind?"
            onSubmit={async (body, attachments) => prepend(await api.createPost(body, undefined, attachments))}
          />
          <ErrorBanner error={error} />
        </>
      }
      ListEmptyComponent={
        loading ? <Loading /> : <Empty text="Nothing here yet. Post something, or add friends and join groups to fill your feed." />
      }
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={refresh} tintColor={colors.accent} />}
      onEndReached={loadMore}
      onEndReachedThreshold={0.4}
      contentContainerStyle={{ paddingBottom: spacing.xl, flexGrow: 1 }}
      ListHeaderComponentStyle={{ marginBottom: spacing.md }}
    />
  );
  if (!wide) return list;
  return (
    <Panes list={list} listWidth="40%">
      {openId ? <PostView key={openId} id={openId} onGone={() => router.setParams({ post: undefined })} /> : <Empty text="Pick a post." />}
    </Panes>
  );
}
