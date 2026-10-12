import { Feather } from '@expo/vector-icons';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, RefreshControl, Text, View } from 'react-native';

import { UserRow } from '@/components/UserRow';
import { Button, Empty, Input, Loading, SectionLabel, Segmented } from '@/components/ui';
import { api } from '@/lib/api';
import { errorMessage } from '@/lib/errors';
import type { User, UserProfile } from '@/lib/types';
import { useDebounced } from '@/lib/useDebounced';
import { useLiveLoad } from '@/lib/useLiveLoad';
import { useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { spacing } from '@/theme';

type Tab = 'friends' | 'requests' | 'find';

export default function FriendsScreen() {
  const [tab, setTab] = useState<Tab>('friends');
  return (
    <View style={{ flex: 1 }}>
      <Segmented
        value={tab}
        onChange={setTab}
        options={[
          { key: 'friends', label: 'Friends' },
          { key: 'requests', label: 'Requests' },
          { key: 'find', label: 'Find people' },
        ]}
      />
      {tab === 'friends' && <FriendList />}
      {tab === 'requests' && <Requests />}
      {tab === 'find' && <Find />}
    </View>
  );
}

const FRIEND_EVENTS = ['friend.accepted'] as const;
const REQUEST_EVENTS = ['friend.request', 'friend.accepted'] as const;

function FriendList() {
  const { colors } = useTheme();
  const [friends, setFriends] = useState<User[] | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const load = useCallback(async () => {
    try {
      setFriends(await api.friends());
    } finally {
      setRefreshing(false);
    }
  }, []);
  useLiveLoad(load, FRIEND_EVENTS);
  if (!friends) return <Loading />;
  return (
    <FlatList
      data={friends}
      keyExtractor={(u) => String(u.id)}
      renderItem={({ item }) => <UserRow user={item} />}
      ListEmptyComponent={<Empty text="No friends yet. Find people and send a request." />}
      refreshControl={
        <RefreshControl
          refreshing={refreshing}
          onRefresh={() => {
            setRefreshing(true);
            void load();
          }}
          tintColor={colors.accent}
        />
      }
      contentContainerStyle={{ flexGrow: 1 }}
    />
  );
}

type RequestItem = { key: string } & ({ header: string } | { user: User; incoming: boolean });

// Incoming requests first, then the ones we sent.
const requestRows = (data: { incoming: User[]; outgoing: User[] }): RequestItem[] =>
  [
    { title: 'Incoming', users: data.incoming, incoming: true },
    { title: 'Sent', users: data.outgoing, incoming: false },
  ].flatMap((s) => [
    { header: s.title, key: `h-${s.title}` },
    ...s.users.map((u) => ({ user: u, incoming: s.incoming, key: `${s.title}-${u.id}` })),
  ]);

function Requests() {
  const [data, setData] = useState<{ incoming: User[]; outgoing: User[] } | null>(null);
  const load = useCallback(async () => setData(await api.friendRequests()), []);
  useLiveLoad(load, REQUEST_EVENTS);
  if (!data) return <Loading />;

  const act = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      await load();
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <FlatList
      data={requestRows(data)}
      keyExtractor={(i) => i.key}
      renderItem={({ item }) =>
        'header' in item ? (
          <SectionLabel style={{ paddingHorizontal: spacing.lg, paddingTop: spacing.lg, paddingBottom: spacing.sm }}>
            {item.header}
          </SectionLabel>
        ) : (
          <UserRow
            user={item.user}
            right={
              item.incoming ? (
                <View style={{ flexDirection: 'row', gap: spacing.sm }}>
                  <Button title="Accept" small onPress={() => act(() => api.acceptFriendRequest(item.user.id))} />
                  <Button title="Decline" small variant="secondary" onPress={() => act(() => api.removeFriend(item.user.id))} />
                </View>
              ) : (
                <Button title="Cancel" small variant="secondary" onPress={() => act(() => api.removeFriend(item.user.id))} />
              )
            }
          />
        )
      }
      ListEmptyComponent={<Empty text="No pending requests." />}
      contentContainerStyle={{ flexGrow: 1 }}
    />
  );
}

// Search results that follow the query after a short pause.
function useUserSearch(query: string) {
  const q = useDebounced(query);
  const [found, setFound] = useState<{ q: string | null; list: UserProfile[] }>({ q: null, list: [] }); // results and the query they answer
  useEffect(() => {
    let current = true;
    api
      .searchUsers(q)
      .then((list) => current && setFound({ q, list }))
      .catch(() => {});
    return () => {
      current = false;
    };
  }, [q]);
  const setStatus = (id: number, friend_status: UserProfile['friend_status']) =>
    setFound((f) => ({ ...f, list: f.list.map((u) => (u.id === id ? { ...u, friend_status } : u)) }));
  return { results: found.list, loading: found.q !== q, setStatus };
}

function Find() {
  const [query, setQuery] = useState('');
  const { results, loading, setStatus } = useUserSearch(query);
  return (
    <View style={{ flex: 1 }}>
      <View style={{ padding: spacing.lg, paddingTop: spacing.sm }}>
        <Input placeholder="Search by name or username" value={query} onChangeText={setQuery} autoCapitalize="none" autoCorrect={false} />
      </View>
      {loading && results.length === 0 ? (
        <Loading />
      ) : (
        <FlatList
          data={results}
          keyExtractor={(u) => String(u.id)}
          renderItem={({ item }) => (
            <UserRow user={item} right={<FriendStatusButton user={item} onStatus={(s) => setStatus(item.id, s)} />} />
          )}
          ListEmptyComponent={<Empty text={query ? 'No one found.' : 'Search for people to add.'} />}
          keyboardShouldPersistTaps="handled"
          contentContainerStyle={{ flexGrow: 1 }}
        />
      )}
    </View>
  );
}

// The one action that fits the friendship state of a search result.
function FriendStatusButton({ user, onStatus }: { user: UserProfile; onStatus: (s: UserProfile['friend_status']) => void }) {
  const { colors } = useTheme();
  const run = async (fn: () => Promise<{ friend_status: string }>) => {
    try {
      onStatus((await fn()).friend_status as UserProfile['friend_status']);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };
  switch (user.friend_status) {
    case 'none':
      return <Button title="Add friend" small onPress={() => run(() => api.sendFriendRequest(user.id))} />;
    case 'outgoing':
      return <Button title="Requested" small variant="secondary" onPress={() => run(() => api.removeFriend(user.id))} />;
    case 'incoming':
      return <Button title="Accept" small onPress={() => run(() => api.acceptFriendRequest(user.id))} />;
    case 'friends':
      return (
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 4 }}>
          <Feather name="check" size={16} color={colors.success} />
          <Text style={{ color: colors.success, fontWeight: '600' }}>Friends</Text>
        </View>
      );
    default:
      return null;
  }
}
