import { Stack, useLocalSearchParams } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { Alert, FlatList, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { PostCard } from '@/components/PostCard';
import { Avatar, Button, Empty, ErrorBanner, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import type { UserProfile } from '@/lib/types';
import { usePosts } from '@/lib/usePosts';
import { colors, spacing } from '@/theme';

export default function UserScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const userId = Number(id);
  const { user: me } = useAuth();
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

  const friendAction = async (fn: () => Promise<{ friend_status: string }>) => {
    try {
      const r = await fn();
      setProfile((p) => (p ? { ...p, friend_status: r.friend_status as UserProfile['friend_status'] } : p));
    } catch (e) {
      Alert.alert('Error', e instanceof Error ? e.message : String(e));
    }
  };

  if (!profile) return error ? <Empty text={error} /> : <Loading />;
  const isMe = profile.id === me?.id;

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
            <Text style={styles.stats}>
              {profile.friend_count ?? 0} friends · {profile.post_count ?? 0} posts
            </Text>
            {!isMe && (
              <View style={styles.actions}>
                {profile.friend_status === 'none' && <Button title="Add friend" onPress={() => friendAction(() => api.sendFriendRequest(profile.id))} />}
                {profile.friend_status === 'outgoing' && <Button title="Cancel request" variant="secondary" onPress={() => friendAction(() => api.removeFriend(profile.id))} />}
                {profile.friend_status === 'incoming' && (
                  <>
                    <Button title="Accept request" onPress={() => friendAction(() => api.acceptFriendRequest(profile.id))} />
                    <Button title="Decline" variant="secondary" onPress={() => friendAction(() => api.removeFriend(profile.id))} />
                  </>
                )}
                {profile.friend_status === 'friends' && (
                  <Button
                    title="Unfriend"
                    variant="ghost"
                    onPress={() =>
                      Alert.alert('Remove friend?', profile.display_name, [
                        { text: 'Cancel', style: 'cancel' },
                        { text: 'Remove', style: 'destructive', onPress: () => friendAction(() => api.removeFriend(profile.id)) },
                      ])
                    }
                  />
                )}
              </View>
            )}
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
  actions: { flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', gap: spacing.sm, marginTop: spacing.lg },
  section: { alignSelf: 'flex-start', fontWeight: '700', color: colors.text, fontSize: 16, marginTop: spacing.xl },
});
