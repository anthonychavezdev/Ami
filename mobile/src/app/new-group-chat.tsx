import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { FriendPicker, toggled } from '@/components/FriendPicker';
import { Button, Empty, Input, Loading } from '@/components/ui';
import { api } from '@/lib/api';
import { toast } from '@/lib/toast';
import type { User } from '@/lib/types';
import { useStyles } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';
import { errorMessage } from '@/lib/errors';

// Pick friends for a new invite-only group chat, or (with ?group=<id>) add
// friends to a group you administer.
export default function NewGroupChatScreen() {
  const styles = useStyles(makeStyles);
  const router = useRouter();
  const { group } = useLocalSearchParams<{ group?: string }>();
  const groupId = group ? Number(group) : null;
  const { friends, error } = useFriendsNotIn(groupId);
  const [name, setName] = useState('');
  const [selected, setSelected] = useState<ReadonlySet<number>>(new Set());
  const [busy, setBusy] = useState(false);

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
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const canSubmit = selected.size > 0 && (groupId !== null || name.trim().length > 0);
  const buttonTitle = groupId
    ? selected.size
      ? `Add ${selected.size}`
      : 'Add members'
    : selected.size
      ? `Create with ${selected.size}`
      : 'Create group chat';

  return (
    <View style={{ flex: 1 }}>
      <Stack.Screen options={{ title: groupId ? 'Add members' : 'New group chat' }} />
      {error ? (
        <Empty text={error} />
      ) : !friends ? (
        <Loading />
      ) : (
        <FriendPicker
          friends={friends}
          selected={selected}
          onToggle={(id) => setSelected((s) => toggled(s, id))}
          header={
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
          empty={groupId ? 'All your friends are already in this group.' : 'Add some friends first; group chats are made of friends.'}
        />
      )}
      {!!friends?.length && (
        <View style={styles.footer}>
          <Button title={buttonTitle} onPress={submit} disabled={!canSubmit} loading={busy} />
        </View>
      )}
    </View>
  );
}

// The caller's friends, minus the members of a group when one is given.
function useFriendsNotIn(groupId: number | null) {
  const [friends, setFriends] = useState<User[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    Promise.all([api.friends(), groupId ? api.group(groupId) : null])
      .then(([all, detail]) => {
        const already = new Set(detail?.members.map((m) => m.id) ?? []);
        setFriends(all.filter((f) => !already.has(f.id)));
      })
      .catch((e) => setError(errorMessage(e)));
  }, [groupId]);
  return { friends, error };
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    header: { padding: spacing.lg, gap: spacing.sm },
    hint: { color: colors.muted, fontSize: 13 },
    section: { fontWeight: '700', color: colors.text, marginTop: spacing.sm },
    footer: {
      position: 'absolute',
      left: 0,
      right: 0,
      bottom: 0,
      padding: spacing.lg,
      backgroundColor: colors.bg,
      borderTopWidth: 1,
      borderTopColor: colors.border,
    },
  });
