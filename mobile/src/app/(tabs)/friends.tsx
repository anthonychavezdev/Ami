import { Feather } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, RefreshControl, Text, View } from 'react-native';

import { UserRow } from '@/components/UserRow';
import { UserView } from '@/components/UserView';
import { Button, Empty, Input, Loading, Panes, SectionLabel, Segmented } from '@/components/ui';
import { api } from '@/lib/api';
import { errorMessage } from '@/lib/errors';
import { useWide } from '@/lib/layout';
import type { FriendStatus, User, UserProfile } from '@/lib/types';
import { useDebounced } from '@/lib/useDebounced';
import { useLiveLoad } from '@/lib/useLiveLoad';
import { useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { socket } from '@/lib/ws';
import { spacing } from '@/theme';

type Tab = 'friends' | 'requests' | 'find';

// How a list opens a person: their profile screen, or, on wide windows, the pane beside the list.
type Opening = { selectedId: string | null; onOpen?: (user: User) => void };

// Friends, requests and people to find. On a wide window the open profile
// sits beside the lists, named by ?user= so a reload or a link (/user/...
// redirects here) lands on it.
export default function FriendsScreen() {
  const router = useRouter();
  const wide = useWide();
  const { user: userParam } = useLocalSearchParams<{ user?: string }>();
  const openId = wide && userParam ? userParam : null;
  const [tab, setTab] = useState<Tab>('friends');
  const opening: Opening = { selectedId: openId, onOpen: wide ? (u) => router.setParams({ user: u.id }) : undefined };

  const lists = (
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
      {tab === 'friends' && <FriendList {...opening} />}
      {tab === 'requests' && <Requests {...opening} />}
      {tab === 'find' && <Find {...opening} />}
    </View>
  );
  if (!wide) return lists;
  return <Panes list={lists}>{openId ? <UserView key={openId} id={openId} embedded /> : <Empty text="Pick a person." />}</Panes>;
}

// A person in one of the lists, opening the way the screen says.
function PersonRow({ user, right, selectedId, onOpen }: { user: User; right?: React.ReactNode } & Opening) {
  return <UserRow user={user} right={right} selected={user.id === selectedId} onPress={onOpen && (() => onOpen(user))} />;
}

const FRIEND_EVENTS = ['friend.changed'] as const;

function FriendList(opening: Opening) {
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
      renderItem={({ item }) => <PersonRow user={item} {...opening} />}
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

function Requests(opening: Opening) {
  const [data, setData] = useState<{ incoming: User[]; outgoing: User[] } | null>(null);
  const load = useCallback(async () => setData(await api.friendRequests()), []);
  useLiveLoad(load, FRIEND_EVENTS);
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
          <PersonRow
            user={item.user}
            {...opening}
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

// Search results that follow the query after a short pause, with each
// result's friendship kept current as either side changes it.
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
  const setStatus = useCallback(
    (id: string, friend_status: FriendStatus) =>
      setFound((f) => ({ ...f, list: f.list.map((u) => (u.id === id ? { ...u, friend_status } : u)) })),
    [],
  );
  useEffect(() => socket.on('friend.changed', (e) => setStatus(e.user.id, e.status)), [setStatus]);
  return { results: found.list, loading: found.q !== q, setStatus };
}

function Find(opening: Opening) {
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
            <PersonRow user={item} {...opening} right={<FriendStatusButton user={item} onStatus={(s) => setStatus(item.id, s)} />} />
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
function FriendStatusButton({ user, onStatus }: { user: UserProfile; onStatus: (s: FriendStatus) => void }) {
  const { colors } = useTheme();
  const run = async (fn: () => Promise<{ friend_status: string }>) => {
    try {
      onStatus((await fn()).friend_status as FriendStatus);
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
