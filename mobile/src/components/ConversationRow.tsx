import { useRouter } from 'expo-router';
import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { UserRow } from './UserRow';
import { GroupAvatar, Row } from './ui';
import { chatRoute, conversationSubtitle } from '@/lib/messages';
import { timeAgo } from '@/lib/time';
import type { Conversation } from '@/lib/types';
import { useStyles } from '@/lib/theme';
import { type ThemeColors, radius } from '@/theme';

// One row of the Messages tab: a friend or a group, the latest message and
// how many are unread.
export function ConversationRow({ conversation, me }: { conversation: Conversation; me: number | undefined }) {
  const styles = useStyles(makeStyles);
  const router = useRouter();
  const subtitle = conversationSubtitle(conversation, me, timeAgo);
  const badge = conversation.unread > 0 && (
    <View style={styles.badge} accessibilityLabel={`${conversation.unread} unread`}>
      <Text style={styles.badgeText}>{conversation.unread > 99 ? '99+' : conversation.unread}</Text>
    </View>
  );
  if (conversation.group) {
    const group = conversation.group;
    return (
      <Row
        left={<GroupAvatar name={group.name} size={44} />}
        title={group.name}
        subtitle={subtitle}
        right={badge}
        onPress={() => router.push(chatRoute({ kind: 'group', id: group.id }))}
      />
    );
  }
  const user = conversation.user!;
  return (
    <UserRow
      user={user}
      subtitle={subtitle}
      right={badge || undefined}
      onPress={() => router.push(chatRoute({ kind: 'user', id: user.id }))}
    />
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    badge: {
      minWidth: 22,
      height: 22,
      borderRadius: radius.pill,
      paddingHorizontal: 6,
      backgroundColor: colors.danger,
      alignItems: 'center',
      justifyContent: 'center',
    },
    badgeText: { color: '#fff', fontSize: 12, fontWeight: '700' },
  });
