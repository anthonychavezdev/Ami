import { Stack, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, RefreshControl } from 'react-native';

import { PostCard } from './PostCard';
import { ProfileHeader } from './ProfileHeader';
import { Empty, Loading } from './ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { useCall } from '@/lib/call';
import { errorMessage } from '@/lib/errors';
import { useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import type { Post, UserProfile } from '@/lib/types';
import { usePosts } from '@/lib/usePosts';
import { spacing } from '@/theme';

// Someone's profile followed by their posts. `embedded` means it sits in the
// pane beside the Friends list, so it leaves the screen title alone.
export function UserView({ id: userId, embedded = false }: { id: number; embedded?: boolean }) {
  const { colors } = useTheme();
  const { user: me } = useAuth();
  const { call } = useCall();
  const router = useRouter();
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fetchPage = useCallback((before?: number) => api.userPosts(userId, before), [userId]);
  const accept = useCallback((p: Post) => p.author.id === userId && p.group_id === null, [userId]);
  const { posts, loading, refreshing, refresh, loadMore, update, remove } = usePosts(fetchPage, accept);

  const loadProfile = useCallback(() => api.user(userId).then(setProfile, (e) => setError(errorMessage(e))), [userId]);
  useEffect(() => {
    void loadProfile();
  }, [loadProfile]);

  // Applies a friendship change (request, accept, remove) to the profile shown.
  const changeFriendship = async (fn: () => Promise<{ friend_status: string }>) => {
    try {
      const r = await fn();
      setProfile((p) => (p ? { ...p, friend_status: r.friend_status as UserProfile['friend_status'] } : p));
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const startCall = (video: boolean) => {
    if (!profile) return;
    void call.startCall(profile, video);
    router.push('/call');
  };

  if (!profile) return error ? <Empty text={error} /> : <Loading />;
  return (
    <>
      {!embedded && <Stack.Screen options={{ title: profile.display_name }} />}
      <FlatList
        data={posts}
        keyExtractor={(p) => String(p.id)}
        renderItem={({ item }) => <PostCard post={item} onChange={update} onDelete={remove} />}
        ListHeaderComponent={
          <ProfileHeader
            profile={profile}
            isMe={profile.id === me?.id}
            error={error}
            onFriendChange={changeFriendship}
            onCall={startCall}
          />
        }
        ListEmptyComponent={loading ? <Loading /> : <Empty text="No posts yet." />}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={() => {
              refresh();
              void loadProfile();
            }}
            tintColor={colors.accent}
          />
        }
        onEndReached={loadMore}
        onEndReachedThreshold={0.4}
        contentContainerStyle={{ paddingBottom: spacing.xl, flexGrow: 1 }}
      />
    </>
  );
}
