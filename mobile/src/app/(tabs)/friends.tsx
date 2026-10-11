import { useFocusEffect } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { Alert, FlatList, RefreshControl, StyleSheet, Text, View } from 'react-native';

import { UserRow } from '@/components/UserRow';
import { Button, Empty, Input, Loading, Segmented } from '@/components/ui';
import { api } from '@/lib/api';
import type { User, UserProfile } from '@/lib/types';
import { socket } from '@/lib/ws';
import { useStyles, useTheme } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

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
  useFocusEffect(
    useCallback(() => {
      void load();
      return socket.on('friend.accepted', () => void load());
    }, [load]),
  );
  if (!friends) return <Loading />;
  return (
    <FlatList
      data={friends}
      keyExtractor={(u) => String(u.id)}
      renderItem={({ item }) => <UserRow user={item} />}
      ListEmptyComponent={<Empty text="No friends yet. Find people and send a request." />}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={() => { setRefreshing(true); void load(); }} tintColor={colors.accent} />}
      contentContainerStyle={{ flexGrow: 1 }}
    />
  );
}

function Requests() {
  const styles = useStyles(makeStyles);
  const [data, setData] = useState<{ incoming: User[]; outgoing: User[] } | null>(null);
  const load = useCallback(async () => setData(await api.friendRequests()), []);
  useFocusEffect(
    useCallback(() => {
      void load();
      const offs = [socket.on('friend.request', () => void load()), socket.on('friend.accepted', () => void load())];
      return () => offs.forEach((off) => off());
    }, [load]),
  );
  if (!data) return <Loading />;

  const act = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      await load();
    } catch (e) {
      Alert.alert('Error', e instanceof Error ? e.message : String(e));
    }
  };

  const sections = [
    { title: 'Incoming', users: data.incoming, incoming: true },
    { title: 'Sent', users: data.outgoing, incoming: false },
  ];

  return (
    <FlatList
      data={sections.flatMap((s) => [{ header: s.title, key: `h-${s.title}` }, ...s.users.map((u) => ({ user: u, incoming: s.incoming, key: `${s.title}-${u.id}` }))])}
      keyExtractor={(i) => i.key}
      renderItem={({ item }) =>
        'header' in item ? (
          <Text style={styles.sectionHeader}>{item.header}</Text>
        ) : (
          <UserRow
            user={item.user!}
            right={
              item.incoming ? (
                <View style={{ flexDirection: 'row', gap: spacing.sm }}>
                  <Button title="Accept" small onPress={() => act(() => api.acceptFriendRequest(item.user!.id))} />
                  <Button title="Decline" small variant="secondary" onPress={() => act(() => api.removeFriend(item.user!.id))} />
                </View>
              ) : (
                <Button title="Cancel" small variant="secondary" onPress={() => act(() => api.removeFriend(item.user!.id))} />
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

function Find() {
  const { colors } = useTheme();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<UserProfile[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const t = setTimeout(async () => {
      setLoading(true);
      try {
        setResults(await api.searchUsers(query));
      } finally {
        setLoading(false);
      }
    }, 250);
    return () => clearTimeout(t);
  }, [query]);

  const setStatus = (id: number, friend_status: UserProfile['friend_status']) =>
    setResults((prev) => prev.map((u) => (u.id === id ? { ...u, friend_status } : u)));

  const action = (u: UserProfile) => {
    const run = async (fn: () => Promise<{ friend_status: string }>) => {
      try {
        const r = await fn();
        setStatus(u.id, r.friend_status as UserProfile['friend_status']);
      } catch (e) {
        Alert.alert('Error', e instanceof Error ? e.message : String(e));
      }
    };
    switch (u.friend_status) {
      case 'none':
        return <Button title="Add friend" small onPress={() => run(() => api.sendFriendRequest(u.id))} />;
      case 'outgoing':
        return <Button title="Requested" small variant="secondary" onPress={() => run(() => api.removeFriend(u.id))} />;
      case 'incoming':
        return <Button title="Accept" small onPress={() => run(() => api.acceptFriendRequest(u.id))} />;
      case 'friends':
        return <Text style={{ color: colors.success, fontWeight: '600' }}>Friends ✓</Text>;
      default:
        return null;
    }
  };

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
          renderItem={({ item }) => <UserRow user={item} right={action(item)} />}
          ListEmptyComponent={<Empty text={query ? 'No one found.' : 'Search for people to add.'} />}
          keyboardShouldPersistTaps="handled"
          contentContainerStyle={{ flexGrow: 1 }}
        />
      )}
    </View>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  sectionHeader: { color: colors.muted, fontWeight: '700', fontSize: 13, textTransform: 'uppercase', paddingHorizontal: spacing.lg, paddingTop: spacing.lg, paddingBottom: spacing.sm },
});
