import { Feather } from '@expo/vector-icons';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { FlatList, StyleSheet, Text, View } from 'react-native';

import { UserRow } from '@/components/UserRow';
import { Button, Empty, Input, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { toast } from '@/lib/toast';
import type { User } from '@/lib/types';
import { useStyles, useTheme } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

// Pick friends for a new invite-only group chat, or (with ?group=<id>) add
// friends to a group you administer.
export default function NewGroupChatScreen() {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const router = useRouter();
  const { group } = useLocalSearchParams<{ group?: string }>();
  const groupId = group ? Number(group) : null;
  const [name, setName] = useState('');
  const [friends, setFriends] = useState<User[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    Promise.all([api.friends(), groupId ? api.group(groupId) : null])
      .then(([all, g]) => {
        const already = new Set(g?.members.map((m) => m.id) ?? []);
        setFriends(all.filter((f) => !already.has(f.id)));
      })
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, [groupId]);

  const toggle = (id: number) =>
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const submit = async () => {
    setBusy(true);
    try {
      const ids = [...selected];
      if (groupId) {
        await api.addGroupMembers(groupId, ids);
        toast.success(ids.length === 1 ? '1 member added' : `${ids.length} members added`);
        router.back();
      } else {
        const r = await api.createGroup(name.trim(), '', { private: true, member_ids: ids });
        router.replace(`/chat/group/${r.group.id}`);
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const canSubmit = selected.size > 0 && (groupId !== null || name.trim().length > 0);

  return (
    <View style={{ flex: 1 }}>
      <Stack.Screen options={{ title: groupId ? 'Add members' : 'New group chat' }} />
      {error ? (
        <Empty text={error} />
      ) : !friends ? (
        <Loading />
      ) : (
        <FlatList
          data={friends}
          keyExtractor={(u) => String(u.id)}
          keyboardShouldPersistTaps="handled"
          ListHeaderComponent={
            <View style={styles.header}>
              {groupId === null && (
                <>
                  <Input placeholder="Group name" value={name} onChangeText={setName} maxLength={60} autoFocus />
                  <Text style={styles.hint}>Invite only: the chat is hidden from Discover, and only people you add can read it.</Text>
                </>
              )}
              <Text style={styles.section}>{friends.length ? 'Friends' : ''}</Text>
            </View>
          }
          renderItem={({ item }) => {
            const on = selected.has(item.id);
            return (
              <UserRow
                user={item}
                onPress={() => toggle(item.id)}
                right={
                  <View style={[styles.check, on && { backgroundColor: colors.accent, borderColor: colors.accent }]} accessibilityLabel={on ? `${item.display_name} selected` : `${item.display_name} not selected`}>
                    {on && <Feather name="check" size={16} color="#fff" />}
                  </View>
                }
              />
            );
          }}
          ListEmptyComponent={<Empty text={groupId ? 'All your friends are already in this group.' : 'Add some friends first; group chats are made of friends.'} />}
          contentContainerStyle={{ flexGrow: 1, paddingBottom: 100 }}
        />
      )}
      {!!friends?.length && (
        <View style={styles.footer}>
          <Button
            title={groupId ? (selected.size ? `Add ${selected.size}` : 'Add members') : selected.size ? `Create with ${selected.size}` : 'Create group chat'}
            onPress={submit}
            disabled={!canSubmit}
            loading={busy}
          />
        </View>
      )}
    </View>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  header: { padding: spacing.lg, gap: spacing.sm },
  hint: { color: colors.muted, fontSize: 13 },
  section: { fontWeight: '700', color: colors.text, marginTop: spacing.sm },
  check: { width: 26, height: 26, borderRadius: 13, borderWidth: 2, borderColor: colors.border, alignItems: 'center', justifyContent: 'center' },
  footer: { position: 'absolute', left: 0, right: 0, bottom: 0, padding: spacing.lg, backgroundColor: colors.bg, borderTopWidth: 1, borderTopColor: colors.border },
});
