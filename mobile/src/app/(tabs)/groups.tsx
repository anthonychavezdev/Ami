import { useFocusEffect, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useState } from 'react';
import { Alert, FlatList, Modal, Pressable, StyleSheet, Text, View } from 'react-native';

import { Button, Empty, Input, Loading, Segmented } from '@/components/ui';
import { api } from '@/lib/api';
import type { Group } from '@/lib/types';
import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

type Tab = 'mine' | 'discover';

export default function GroupsScreen() {
  const styles = useStyles(makeStyles);
  const [tab, setTab] = useState<Tab>('mine');
  const [creating, setCreating] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  return (
    <View style={{ flex: 1 }}>
      <Segmented
        value={tab}
        onChange={setTab}
        options={[
          { key: 'mine', label: 'My groups' },
          { key: 'discover', label: 'Discover' },
        ]}
      />
      {tab === 'mine' ? <MyGroups key={reloadKey} /> : <Discover />}
      <Pressable style={styles.fab} onPress={() => setCreating(true)}>
        <Text style={styles.fabText}>＋</Text>
      </Pressable>
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
}

function GroupRow({ group }: { group: Group }) {
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const router = useRouter();
  return (
    <Pressable onPress={() => router.push(`/group/${group.id}`)} style={({ pressed }) => [styles.row, pressed && { opacity: 0.7 }]}>
      <View style={styles.groupIcon}>
        <Text style={{ fontSize: 22 }}>👥</Text>
      </View>
      <View style={{ flex: 1 }}>
        <Text style={styles.name}>{group.name}</Text>
        <Text style={styles.sub} numberOfLines={1}>
          {group.member_count} {group.member_count === 1 ? 'member' : 'members'}
          {group.my_role ? ` · ${group.my_role}` : ''}
          {group.description ? ` · ${group.description}` : ''}
        </Text>
      </View>
      <Text style={{ color: colors.muted }}>›</Text>
    </Pressable>
  );
}

function MyGroups() {
  const [groups, setGroups] = useState<Group[] | null>(null);
  useFocusEffect(
    useCallback(() => {
      api.myGroups().then(setGroups).catch(() => setGroups([]));
    }, []),
  );
  if (!groups) return <Loading />;
  return (
    <FlatList
      data={groups}
      keyExtractor={(g) => String(g.id)}
      renderItem={({ item }) => <GroupRow group={item} />}
      ListEmptyComponent={<Empty text="You haven't joined any groups. Discover some, or create your own with ＋." />}
      contentContainerStyle={{ flexGrow: 1, paddingBottom: 90 }}
    />
  );
}

function Discover() {
  const [query, setQuery] = useState('');
  const [groups, setGroups] = useState<Group[] | null>(null);
  useEffect(() => {
    const t = setTimeout(() => api.discoverGroups(query).then(setGroups).catch(() => setGroups([])), 250);
    return () => clearTimeout(t);
  }, [query]);
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
          renderItem={({ item }) => <GroupRow group={item} />}
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
  const [busy, setBusy] = useState(false);
  const router = useRouter();

  const create = async () => {
    setBusy(true);
    try {
      const r = await api.createGroup(name.trim(), description.trim());
      setName('');
      setDescription('');
      onCreated();
      router.push(`/group/${r.group.id}`);
    } catch (e) {
      Alert.alert('Could not create group', e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <View style={styles.modal}>
        <Text style={styles.modalTitle}>New group</Text>
        <Text style={styles.hint}>You'll be the owner and admin. Anyone can find and join it.</Text>
        <Input placeholder="Group name" value={name} onChangeText={setName} maxLength={60} />
        <Input placeholder="Description (optional)" value={description} onChangeText={setDescription} multiline style={{ minHeight: 80, textAlignVertical: 'top' }} maxLength={500} />
        <Button title="Create group" onPress={create} loading={busy} disabled={!name.trim()} />
        <Button title="Cancel" variant="ghost" onPress={onClose} />
      </View>
    </Modal>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.md, paddingHorizontal: spacing.lg, paddingVertical: spacing.md, backgroundColor: colors.card, borderBottomWidth: 1, borderBottomColor: colors.border },
  groupIcon: { width: 44, height: 44, borderRadius: radius.md, backgroundColor: colors.accentSoft, alignItems: 'center', justifyContent: 'center' },
  name: { fontWeight: '700', color: colors.text, fontSize: 15 },
  sub: { color: colors.muted, fontSize: 13 },
  fab: { position: 'absolute', right: spacing.xl, bottom: spacing.xl, width: 56, height: 56, borderRadius: 28, backgroundColor: colors.accent, alignItems: 'center', justifyContent: 'center', elevation: 6, shadowColor: '#000', shadowOpacity: 0.2, shadowRadius: 8, shadowOffset: { width: 0, height: 4 } },
  fabText: { color: '#fff', fontSize: 28, lineHeight: 32 },
  modal: { flex: 1, padding: spacing.xl, gap: spacing.md, backgroundColor: colors.bg },
  modalTitle: { fontSize: 24, fontWeight: '800', color: colors.text },
  hint: { color: colors.muted, marginBottom: spacing.sm },
});
