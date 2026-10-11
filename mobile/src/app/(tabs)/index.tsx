import React, { useCallback } from 'react';
import { FlatList, RefreshControl } from 'react-native';

import { Composer } from '@/components/Composer';
import { PostCard } from '@/components/PostCard';
import { Empty, ErrorBanner, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { usePosts } from '@/lib/usePosts';
import { useTheme } from '@/lib/theme';
import { spacing } from '@/theme';

export default function FeedScreen() {
  const { colors } = useTheme();
  const fetchPage = useCallback((before?: number) => api.feed(before), []);
  const { posts, loading, refreshing, error, refresh, loadMore, update, remove, prepend } = usePosts(fetchPage);

  return (
    <FlatList
      data={posts}
      keyExtractor={(p) => String(p.id)}
      renderItem={({ item }) => <PostCard post={item} onChange={update} onDelete={remove} />}
      ListHeaderComponent={
        <>
          <Composer rich placeholder="What's on your mind?" onSubmit={async (body, attachments) => prepend(await api.createPost(body, undefined, attachments))} />
          <ErrorBanner error={error} />
        </>
      }
      ListEmptyComponent={loading ? <Loading /> : <Empty text="Nothing here yet. Post something, or add friends and join groups to fill your feed." />}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={refresh} tintColor={colors.accent} />}
      onEndReached={loadMore}
      onEndReachedThreshold={0.4}
      contentContainerStyle={{ paddingBottom: spacing.xl, flexGrow: 1 }}
      ListHeaderComponentStyle={{ marginBottom: spacing.md }}
    />
  );
}
