import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { PostCard } from '@/components/PostCard';
import { Avatar, Button, Empty, ErrorBanner, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { confirmAction } from '@/lib/confirm';
import { useAuth } from '@/lib/auth';
import { useCall } from '@/lib/call';
import { toast } from '@/lib/toast';
import type { Post, UserProfile } from '@/lib/types';
import { usePosts } from '@/lib/usePosts';
import { useStyles, useTheme } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

export default function UserScreen() {
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const { id } = useLocalSearchParams<{ id: string }>();
  const userId = Number(id);
  const { user: me } = useAuth();
  const { call } = useCall();
  const router = useRouter();
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchPage = useCallback((before?: number) => api.userPosts(userId, before), [userId]);
  const accept = useCallback((p: Post) => p.author.id === userId && p.group_id === null, [userId]);
  const { posts, loading, refreshing, refresh, loadMore, update, remove } = usePosts(fetchPage, accept);

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
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  const startCall = (video: boolean) => {
    if (!profile) return;
    void call.startCall(profile, video);
    router.push('/call');
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
                  <>
                    <Button title="Message" onPress={() => router.push(`/chat/${profile.id}`)} />
                    <Button title="Call" icon="phone" variant="secondary" onPress={() => startCall(false)} />
                    <Button title="Video" icon="video" variant="secondary" onPress={() => startCall(true)} />
                    <Button
                      title="Unfriend"
                      variant="ghost"
                      onPress={() =>
                        confirmAction('Remove friend?', profile.display_name, 'Remove', () => friendAction(() => api.removeFriend(profile.id)))
                      }
                    />
                  </>
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

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  header: { alignItems: 'center', padding: spacing.xl, gap: spacing.xs },
  name: { fontSize: 22, fontWeight: '800', color: colors.text, marginTop: spacing.sm },
  username: { color: colors.muted },
  stats: { color: colors.muted, marginTop: spacing.xs },
  actions: { flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', gap: spacing.sm, marginTop: spacing.lg },
  section: { alignSelf: 'flex-start', fontWeight: '700', color: colors.text, fontSize: 16, marginTop: spacing.xl },
});
