import { Feather } from '@expo/vector-icons';
import { useFocusEffect, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { FlatList, Modal, StyleSheet, Switch, Text, View } from 'react-native';

import { GroupView } from '@/components/GroupView';
import { Button, Empty, Fab, Input, Loading, Panes, Row, Segmented } from '@/components/ui';
import { api } from '@/lib/api';
import { errorMessage } from '@/lib/errors';
import { useWide } from '@/lib/layout';
import type { Group } from '@/lib/types';
import { useDebounced } from '@/lib/useDebounced';
import { useStyles, useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { radius, spacing, type ThemeColors } from '@/theme';

type Tab = 'mine' | 'discover';

// How a list opens a group: the group screen, or, on wide windows, the pane beside the list.
type Opening = { selectedId: string | null; onOpen?: (group: Group) => void };

// My groups and public groups to discover. On a wide window the open group
// sits beside the list, named by ?group= so a reload or a link (/group/...
// redirects here) lands on it.
export default function GroupsScreen() {
  const router = useRouter();
  const wide = useWide();
  const { group: groupParam } = useLocalSearchParams<{ group?: string }>();
  const openId = wide && groupParam ? groupParam : null;
  const [tab, setTab] = useState<Tab>('mine');
  const [creating, setCreating] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const opening: Opening = { selectedId: openId, onOpen: wide ? (g) => router.setParams({ group: g.id }) : undefined };

  const lists = (
    <View style={{ flex: 1 }}>
      <Segmented
        value={tab}
        onChange={setTab}
        options={[
          { key: 'mine', label: 'My groups' },
          { key: 'discover', label: 'Discover' },
        ]}
      />
      {tab === 'mine' ? <MyGroups key={reloadKey} {...opening} /> : <Discover {...opening} />}
      <Fab label="New group" onPress={() => setCreating(true)} />
      <CreateGroupModal
        visible={creating}
        onClose={() => setCreating(false)}
        onCreated={() => {
          setCreating(false);
          setTab('mine');
          setReloadKey((k) => k + 1);
        }}
      />
    </View>
  );
  if (!wide) return lists;
  return (
    <Panes list={lists}>
      {openId ? (
        <GroupView
          key={openId}
          id={openId}
          embedded
          onGone={() => {
            router.setParams({ group: undefined });
            setReloadKey((k) => k + 1);
          }}
        />
      ) : (
        <Empty text="Pick a group." />
      )}
    </Panes>
  );
}

function GroupRow({ group, selectedId, onOpen }: { group: Group } & Opening) {
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const router = useRouter();
  const members = `${group.member_count} ${group.member_count === 1 ? 'member' : 'members'}`;
  const subtitle = members + (group.my_role ? ` · ${group.my_role}` : '') + (group.description ? ` · ${group.description}` : '');
  return (
    <Row
      left={
        <View style={styles.groupIcon}>
          <Feather name="users" size={22} color={colors.accent} />
        </View>
      }
      title={group.name}
      subtitle={subtitle}
      right={<Feather name="chevron-right" size={20} color={colors.muted} />}
      selected={group.id === selectedId}
      onPress={() => (onOpen ? onOpen(group) : router.push(`/group/${group.id}`))}
    />
  );
}

function MyGroups(opening: Opening) {
  const [groups, setGroups] = useState<Group[] | null>(null);
  useFocusEffect(
    useCallback(() => {
      api
        .myGroups()
        .then(setGroups)
        .catch(() => setGroups([]));
    }, []),
  );
  if (!groups) return <Loading />;
  return (
    <FlatList
      data={groups}
      keyExtractor={(g) => String(g.id)}
      renderItem={({ item }) => <GroupRow group={item} {...opening} />}
      ListEmptyComponent={<Empty text="You haven't joined any groups. Discover some, or create your own with the + button." />}
      contentContainerStyle={{ flexGrow: 1, paddingBottom: 90 }}
    />
  );
}

function Discover(opening: Opening) {
  const [query, setQuery] = useState('');
  const q = useDebounced(query);
  const [groups, setGroups] = useState<Group[] | null>(null);
  useEffect(() => {
    api
      .discoverGroups(q)
      .then(setGroups)
      .catch(() => setGroups([]));
  }, [q]);
  return (
    <View style={{ flex: 1 }}>
      <View style={{ padding: spacing.lg, paddingTop: spacing.sm }}>
        <Input placeholder="Search groups" value={query} onChangeText={setQuery} />
      </View>
      {!groups ? (
        <Loading />
      ) : (
        <FlatList
          data={groups}
          keyExtractor={(g) => String(g.id)}
          renderItem={({ item }) => <GroupRow group={item} {...opening} />}
          ListEmptyComponent={<Empty text="No groups found." />}
          keyboardShouldPersistTaps="handled"
          contentContainerStyle={{ flexGrow: 1, paddingBottom: 90 }}
        />
      )}
    </View>
  );
}

function CreateGroupModal({ visible, onClose, onCreated }: { visible: boolean; onClose: () => void; onCreated: () => void }) {
  const styles = useStyles(makeStyles);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [inviteOnly, setInviteOnly] = useState(false);
  const [busy, setBusy] = useState(false);
  const router = useRouter();
  const { colors } = useTheme();

  const create = async () => {
    setBusy(true);
    try {
      const r = await api.createGroup(name.trim(), description.trim(), { private: inviteOnly });
      setName('');
      setDescription('');
      setInviteOnly(false);
      onCreated();
      router.push(`/group/${r.group.id}`);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <View style={styles.modal}>
        <Text style={styles.modalTitle}>New group</Text>
        <Text style={styles.hint}>
          You&apos;ll be the owner and admin. {inviteOnly ? 'Only people you add can find it.' : 'Anyone can find and join it.'}
        </Text>
        <Input placeholder="Group name" value={name} onChangeText={setName} maxLength={60} />
        <Input
          placeholder="Description (optional)"
          value={description}
          onChangeText={setDescription}
          multiline
          style={{ minHeight: 80, textAlignVertical: 'top' }}
          maxLength={500}
        />
        <View style={styles.switchRow}>
          <Text style={styles.switchLabel}>Invite only</Text>
          <Switch value={inviteOnly} onValueChange={setInviteOnly} trackColor={{ true: colors.accent }} accessibilityLabel="Invite only" />
        </View>
        <Button title="Create group" onPress={create} loading={busy} disabled={!name.trim()} />
        <Button title="Cancel" variant="ghost" onPress={onClose} />
      </View>
    </Modal>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    groupIcon: {
      width: 44,
      height: 44,
      borderRadius: radius.md,
      backgroundColor: colors.accentSoft,
      alignItems: 'center',
      justifyContent: 'center',
    },
    modal: { flex: 1, padding: spacing.xl, gap: spacing.md, backgroundColor: colors.bg },
    modalTitle: { fontSize: 24, fontWeight: '800', color: colors.text },
    hint: { color: colors.muted, marginBottom: spacing.sm },
    switchRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingVertical: spacing.xs },
    switchLabel: { color: colors.text, fontSize: 16 },
  });
