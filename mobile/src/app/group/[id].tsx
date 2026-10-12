import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { Composer } from '@/components/Composer';
import { PostCard } from '@/components/PostCard';
import { UserRow } from '@/components/UserRow';
import { Button, Empty, ErrorBanner, Loading, Segmented } from '@/components/ui';
import { api } from '@/lib/api';
import { confirmAction } from '@/lib/confirm';
import { useAuth } from '@/lib/auth';
import { toast } from '@/lib/toast';
import type { Group, GroupMember, Post } from '@/lib/types';
import { usePosts } from '@/lib/usePosts';
import { useStyles, useTheme } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

type Tab = 'posts' | 'members';

export default function GroupScreen() {
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const { id } = useLocalSearchParams<{ id: string }>();
  const groupId = Number(id);
  const { user: me } = useAuth();
  const router = useRouter();
  const [group, setGroup] = useState<Group | null>(null);
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>('posts');

  const fetchPage = useCallback((before?: number) => api.groupPosts(groupId, before), [groupId]);
  const accept = useCallback((p: Post) => p.group_id === groupId, [groupId]);
  const { posts, loading, refreshing, refresh, loadMore, update, remove, prepend } = usePosts(fetchPage, accept);

  const apply = (r: { group: Group; members: GroupMember[] }) => {
    setGroup(r.group);
    setMembers(r.members);
  };
  const load = useCallback(async () => {
    try {
      apply(await api.group(groupId));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [groupId]);
  useEffect(() => {
    void load();
  }, [load]);

  const run = async (fn: () => Promise<{ group: Group; members: GroupMember[] }>) => {
    try {
      apply(await fn());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  const deleteGroup = () =>
    confirmAction('Delete group?', 'All its posts will be removed.', 'Delete', async () => {
      try {
        await api.deleteGroup(groupId);
        router.back();
      } catch (e) {
        toast.error(e instanceof Error ? e.message : String(e));
      }
    });

  if (!group) return error ? <Empty text={error} /> : <Loading />;
  const isAdmin = group.my_role === 'admin';
  const isMember = group.my_role !== '';

  const header = (
    <View>
      <View style={styles.header}>
        <Text style={styles.name}>{group.name}</Text>
        {!!group.description && <Text style={styles.desc}>{group.description}</Text>}
        <Text style={styles.meta}>
          {group.member_count} {group.member_count === 1 ? 'member' : 'members'}
          {isAdmin ? ' · you are the admin' : isMember ? ' · member' : ''}
        </Text>
        <View style={styles.actions}>
          {!isMember && <Button title="Join group" onPress={() => run(() => api.joinGroup(groupId))} />}
          {isMember && !isAdmin && <Button title="Leave" variant="secondary" onPress={() => run(() => api.leaveGroup(groupId))} />}
          {isAdmin && <Button title="Delete group" variant="danger" onPress={deleteGroup} />}
        </View>
      </View>
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
          <Composer rich placeholder={`Post in ${group.name}`} onSubmit={async (body, attachments) => prepend(await api.createPost(body, groupId, attachments))} />
        </View>
      )}
    </View>
  );

  if (tab === 'members') {
    return (
      <>
        <Stack.Screen options={{ title: group.name }} />
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
                      confirmAction('Remove member?', item.display_name, 'Remove', () => run(() => api.removeGroupMember(groupId, item.id)))
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
      <Stack.Screen options={{ title: group.name }} />
      <FlatList
        data={posts}
        keyExtractor={(p) => String(p.id)}
        ListHeaderComponent={header}
        renderItem={({ item }) => <PostCard post={item} onChange={update} onDelete={remove} canDelete={isAdmin} showGroup={false} />}
        ListEmptyComponent={loading ? <Loading /> : <Empty text={isMember ? 'No posts yet. Start the conversation.' : 'No posts yet.'} />}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { refresh(); void load(); }} tintColor={colors.accent} />}
        onEndReached={loadMore}
        onEndReachedThreshold={0.4}
        contentContainerStyle={{ paddingBottom: spacing.xl, flexGrow: 1 }}
      />
    </>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  header: { padding: spacing.xl, paddingBottom: spacing.sm, gap: spacing.xs },
  name: { fontSize: 24, fontWeight: '800', color: colors.text },
  desc: { color: colors.text, fontSize: 15, lineHeight: 21 },
  meta: { color: colors.muted },
  actions: { flexDirection: 'row', gap: spacing.sm, marginTop: spacing.md },
});
