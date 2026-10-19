import { Stack, useRouter } from 'expo-router';
import React, { useCallback, useState } from 'react';
import { FlatList, RefreshControl, View } from 'react-native';

import { Composer } from './Composer';
import { GroupHeader } from './GroupHeader';
import { PostCard } from './PostCard';
import { UserRow } from './UserRow';
import { Button, Empty, ErrorBanner, Loading, Segmented } from './ui';
import { api } from '@/lib/api';
import { confirmAction } from '@/lib/confirm';
import { useAuth } from '@/lib/auth';
import { useCall } from '@/lib/call';
import { toast } from '@/lib/toast';
import type { Post } from '@/lib/types';
import { useGroup } from '@/lib/useGroup';
import { usePosts } from '@/lib/usePosts';
import { useTheme } from '@/lib/theme';
import { spacing } from '@/theme';
import { errorMessage } from '@/lib/errors';

type Tab = 'posts' | 'members';

// A group: its header and actions, then its posts or its members. `embedded`
// is the pane of the wide Groups view, which has no navigator header to set;
// `onGone` runs once the group has been deleted.
export function GroupView({ id: groupId, embedded = false, onGone }: { id: string; embedded?: boolean; onGone: () => void }) {
  const { colors } = useTheme();
  const { user: me } = useAuth();
  const { call } = useCall();
  const router = useRouter();
  const { group, members, error, reload, change } = useGroup(groupId);
  const [tab, setTab] = useState<Tab>('posts');

  const fetchPage = useCallback((before?: string) => api.groupPosts(groupId, before), [groupId]);
  const accept = useCallback((p: Post) => p.group_id === groupId, [groupId]);
  const { posts, loading, refreshing, refresh, loadMore, update, remove, prepend } = usePosts(fetchPage, accept);

  const deleteGroup = () =>
    confirmAction('Delete group?', 'All its posts will be removed.', 'Delete', async () => {
      try {
        await api.deleteGroup(groupId);
        onGone();
      } catch (e) {
        toast.error(errorMessage(e));
      }
    });

  const startCall = (video: boolean) => {
    if (!group) return;
    void call.joinGroupCall({ id: group.id, name: group.name }, video);
    router.push('/call');
  };

  if (!group) return error ? <Empty text={error} /> : <Loading />;
  const isAdmin = group.my_role === 'admin';
  const isMember = group.my_role !== '';

  const header = (
    <View>
      <GroupHeader
        group={group}
        onJoin={() => change(() => api.joinGroup(groupId))}
        onLeave={() => change(() => api.leaveGroup(groupId))}
        onDelete={deleteGroup}
        onCall={startCall}
      />
      <ErrorBanner error={error} />
      <Segmented
        value={tab}
        onChange={setTab}
        options={[
          { key: 'posts', label: 'Posts' },
          { key: 'members', label: 'Members' },
        ]}
      />
      {tab === 'posts' && isMember && (
        <View style={{ marginBottom: spacing.md }}>
          <Composer
            rich
            placeholder={`Post in ${group.name}`}
            onSubmit={async (body, attachments) => prepend(await api.createPost(body, groupId, attachments))}
          />
        </View>
      )}
    </View>
  );

  if (tab === 'members') {
    return (
      <>
        {!embedded && <Stack.Screen options={{ title: group.name }} />}
        <FlatList
          data={members}
          keyExtractor={(m) => String(m.id)}
          ListHeaderComponent={header}
          renderItem={({ item }) => (
            <UserRow
              user={item}
              subtitle={item.role === 'admin' ? 'Admin (owner)' : `@${item.username}`}
              right={
                isAdmin && item.id !== me?.id ? (
                  <Button
                    title="Remove"
                    small
                    variant="secondary"
                    onPress={() =>
                      confirmAction('Remove member?', item.display_name, 'Remove', () =>
                        change(() => api.removeGroupMember(groupId, item.id)),
                      )
                    }
                  />
                ) : null
              }
            />
          )}
        />
      </>
    );
  }

  return (
    <>
      {!embedded && <Stack.Screen options={{ title: group.name }} />}
      <FlatList
        data={posts}
        keyExtractor={(p) => String(p.id)}
        ListHeaderComponent={header}
        renderItem={({ item }) => <PostCard post={item} onChange={update} onDelete={remove} canDelete={isAdmin} showGroup={false} />}
        ListEmptyComponent={loading ? <Loading /> : <Empty text={isMember ? 'No posts yet. Start the conversation.' : 'No posts yet.'} />}
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={() => {
              refresh();
              void reload();
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
