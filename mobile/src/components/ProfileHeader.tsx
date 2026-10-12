import { useRouter } from 'expo-router';
import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { Avatar, Button, ErrorBanner } from './ui';
import { api } from '@/lib/api';
import { confirmAction } from '@/lib/confirm';
import { useStyles } from '@/lib/theme';
import type { UserProfile } from '@/lib/types';
import { spacing, type ThemeColors } from '@/theme';

// The top of someone's profile: picture, names, bio, counts and what you can
// do with them (befriend, message, call).
export function ProfileHeader({ profile, isMe, error, onFriendChange, onCall }: { profile: UserProfile; isMe: boolean; error: string | null; onFriendChange: (fn: () => Promise<{ friend_status: string }>) => void; onCall: (video: boolean) => void }) {
  const styles = useStyles(makeStyles);
  return (
    <View style={styles.header}>
      <Avatar user={profile} size={72} />
      <Text style={styles.name}>{profile.display_name}</Text>
      <Text style={styles.username}>
        @{profile.username}
        {profile.pronouns ? ` · ${profile.pronouns}` : ''}
      </Text>
      {!!profile.bio && <Text style={styles.bio}>{profile.bio}</Text>}
      <Text style={styles.stats}>
        {profile.friend_count ?? 0} friends · {profile.post_count ?? 0} posts
      </Text>
      {!isMe && (
        <View style={styles.actions}>
          <FriendButtons profile={profile} onChange={onFriendChange} onCall={onCall} />
        </View>
      )}
      <ErrorBanner error={error} />
      <Text style={styles.section}>Posts</Text>
    </View>
  );
}

// The buttons for each friendship state.
function FriendButtons({ profile, onChange, onCall }: { profile: UserProfile; onChange: (fn: () => Promise<{ friend_status: string }>) => void; onCall: (video: boolean) => void }) {
  const router = useRouter();
  switch (profile.friend_status) {
    case 'none':
      return <Button title="Add friend" onPress={() => onChange(() => api.sendFriendRequest(profile.id))} />;
    case 'outgoing':
      return <Button title="Cancel request" variant="secondary" onPress={() => onChange(() => api.removeFriend(profile.id))} />;
    case 'incoming':
      return (
        <>
          <Button title="Accept request" onPress={() => onChange(() => api.acceptFriendRequest(profile.id))} />
          <Button title="Decline" variant="secondary" onPress={() => onChange(() => api.removeFriend(profile.id))} />
        </>
      );
    case 'friends':
      return (
        <>
          <Button title="Message" onPress={() => router.push(`/chat/${profile.id}`)} />
          <Button title="Call" icon="phone" variant="secondary" onPress={() => onCall(false)} />
          <Button title="Video" icon="video" variant="secondary" onPress={() => onCall(true)} />
          <Button title="Unfriend" variant="ghost" onPress={() => confirmAction('Remove friend?', profile.display_name, 'Remove', () => onChange(() => api.removeFriend(profile.id)))} />
        </>
      );
    default:
      return null;
  }
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  header: { alignItems: 'center', padding: spacing.xl, gap: spacing.xs },
  name: { fontSize: 22, fontWeight: '800', color: colors.text, marginTop: spacing.sm },
  username: { color: colors.muted },
  bio: { color: colors.text, fontSize: 15, lineHeight: 21, textAlign: 'center', marginTop: spacing.sm, paddingHorizontal: spacing.lg },
  stats: { color: colors.muted, marginTop: spacing.xs },
  actions: { flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', gap: spacing.sm, marginTop: spacing.lg },
  section: { alignSelf: 'flex-start', fontWeight: '700', color: colors.text, fontSize: 16, marginTop: spacing.xl },
});
