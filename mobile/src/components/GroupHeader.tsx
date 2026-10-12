import { useRouter } from 'expo-router';
import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { Button } from './ui';
import { useStyles } from '@/lib/theme';
import type { Group } from '@/lib/types';
import { spacing, type ThemeColors } from '@/theme';

// Name, description, membership line and the actions open to the viewer.
export function GroupHeader({ group, onJoin, onLeave, onDelete, onCall }: { group: Group; onJoin: () => void; onLeave: () => void; onDelete: () => void; onCall: (video: boolean) => void }) {
  const styles = useStyles(makeStyles);
  const router = useRouter();
  const isAdmin = group.my_role === 'admin';
  const isMember = group.my_role !== '';
  return (
    <View style={styles.header}>
      <Text style={styles.name}>{group.name}</Text>
      {!!group.description && <Text style={styles.desc}>{group.description}</Text>}
      <Text style={styles.meta}>{membershipLine(group)}</Text>
      <View style={styles.actions}>
        {!isMember && !group.private && <Button title="Join group" onPress={onJoin} />}
        {isMember && <Button title="Chat" icon="message-circle" variant="secondary" onPress={() => router.push(`/chat/group/${group.id}`)} />}
        {isMember && <Button title="Call" icon="phone" variant="secondary" onPress={() => onCall(false)} />}
        {isMember && <Button title="Video" icon="video" variant="secondary" onPress={() => onCall(true)} />}
        {isAdmin && <Button title="Add members" icon="user-plus" variant="secondary" onPress={() => router.push(`/new-group-chat?group=${group.id}`)} />}
        {isMember && !isAdmin && <Button title="Leave" variant="secondary" onPress={onLeave} />}
        {isAdmin && <Button title="Delete group" variant="danger" onPress={onDelete} />}
      </View>
    </View>
  );
}

function membershipLine(group: Group) {
  const count = `${group.member_count} ${group.member_count === 1 ? 'member' : 'members'}`;
  const privacy = group.private ? ' · invite only' : '';
  const role = group.my_role === 'admin' ? ' · you are the admin' : group.my_role === 'member' ? ' · member' : '';
  return count + privacy + role;
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  header: { padding: spacing.xl, paddingBottom: spacing.sm, gap: spacing.xs },
  name: { fontSize: 24, fontWeight: '800', color: colors.text },
  desc: { color: colors.text, fontSize: 15, lineHeight: 21 },
  meta: { color: colors.muted },
  actions: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.md },
});
