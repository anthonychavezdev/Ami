import { Stack, useLocalSearchParams } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { PostCard } from '@/components/PostCard';
import { Avatar, Empty, ErrorBanner, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import type { UserProfile } from '@/lib/types';
import { usePosts } from '@/lib/usePosts';
import { colors, spacing } from '@/theme';

export default function UserScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const userId = Number(id);
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchPage = useCallback((before?: number) => api.userPosts(userId, before), [userId]);
  const { posts, loading, refreshing, refresh, loadMore, update, remove } = usePosts(fetchPage);

  const loadProfile = useCallback(async () => {
    try {
      setProfile(await api.user(userId));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [userId]);

  useEffect(() => {
    void loadProfile();
  }, [loadProfile]);

  if (!profile) return error ? <Empty text={error} /> : <Loading />;

  return (
    <>
      <Stack.Screen options={{ title: profile.display_name }} />
      <FlatList
        data={posts}
        keyExtractor={(p) => String(p.id)}
        renderItem={({ item }) => <PostCard post={item} onChange={update} onDelete={remove} />}
        ListHeaderComponent={
          <View style={styles.header}>
            <Avatar user={profile} size={72} />
            <Text style={styles.name}>{profile.display_name}</Text>
            <Text style={styles.username}>@{profile.username}</Text>
            <Text style={styles.stats}>{profile.post_count ?? 0} posts</Text>
            <ErrorBanner error={error} />
            <Text style={styles.section}>Posts</Text>
          </View>
        }
        ListEmptyComponent={loading ? <Loading /> : <Empty text="No posts yet." />}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { refresh(); void loadProfile(); }} tintColor={colors.accent} />}
        onEndReached={loadMore}
        onEndReachedThreshold={0.4}
        contentContainerStyle={{ paddingBottom: spacing.xl, flexGrow: 1 }}
      />
    </>
  );
}

const styles = StyleSheet.create({
  header: { alignItems: 'center', padding: spacing.xl, gap: spacing.xs },
  name: { fontSize: 22, fontWeight: '800', color: colors.text, marginTop: spacing.sm },
  username: { color: colors.muted },
  stats: { color: colors.muted, marginTop: spacing.xs },
  section: { alignSelf: 'flex-start', fontWeight: '700', color: colors.text, fontSize: 16, marginTop: spacing.xl },
});
