import { Feather } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Linking, Pressable, StyleSheet, Text, View } from 'react-native';

import { Cover } from './Cover';
import { MediaViewer } from './MediaViewer';
import { Avatar, Button, ErrorBanner } from './ui';
import { api } from '@/lib/api';
import { confirmAction } from '@/lib/confirm';
import { PostMarkdown } from '@/lib/markdown';
import { useStyles, useTheme } from '@/lib/theme';
import type { Attachment, UserProfile } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

// The top of someone's profile: cover, picture, names, bio (Markdown),
// interests, links, counts and what you can do with them (befriend, message, call).
// Tapping the cover or the picture opens it in the fullscreen viewer, with
// Save and the reactions to the picture itself.
export function ProfileHeader({
  profile,
  isMe,
  error,
  onFriendChange,
  onCall,
}: {
  profile: UserProfile;
  isMe: boolean;
  error: string | null;
  onFriendChange: (fn: () => Promise<{ friend_status: string }>) => void;
  onCall: (video: boolean) => void;
}) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const router = useRouter();
  const pictures: Attachment[] = [profile.cover_url, profile.avatar_url]
    .filter((url): url is string => !!url)
    .map((url) => ({ type: 'image', url }));
  const [open, setOpen] = useState<number | null>(null); // which of them the viewer shows
  return (
    <View>
      {!!profile.cover_url && (
        <Pressable onPress={() => setOpen(0)} accessibilityLabel="Open the cover picture">
          <Cover url={profile.cover_url} crop={profile.cover_crop} />
        </Pressable>
      )}
      <View style={styles.header}>
        <Pressable
          style={profile.cover_url ? styles.overCover : undefined}
          onPress={profile.avatar_url ? () => setOpen(pictures.length - 1) : undefined}
          accessibilityLabel={profile.avatar_url ? 'Open the profile picture' : undefined}
        >
          <Avatar user={profile} size={72} />
        </Pressable>
        <MediaViewer items={pictures} index={open} accent={colors.accent} onClose={() => setOpen(null)} />
        <Text style={styles.name}>{profile.display_name}</Text>
        <Text style={styles.username}>
          @{profile.username}
          {profile.pronouns ? ` · ${profile.pronouns}` : ''}
        </Text>
        {!!profile.bio && (
          <View style={styles.bio}>
            <PostMarkdown>{profile.bio}</PostMarkdown>
          </View>
        )}
        {!!profile.interests?.length && (
          <View style={styles.chips}>
            {profile.interests.map((tag) => (
              <Pressable
                key={tag}
                onPress={() => router.push({ pathname: '/friends', params: { q: tag } })}
                style={({ pressed }) => [styles.chip, pressed && { opacity: 0.7 }]}
                accessibilityLabel={`Find people into ${tag}`}
              >
                <Text style={styles.chipText}>{tag}</Text>
              </Pressable>
            ))}
          </View>
        )}
        {!!profile.links?.length && (
          <View style={styles.chips}>
            {profile.links.map((l) => (
              <Pressable
                key={l.url}
                onPress={() => Linking.openURL(l.url).catch(() => {})}
                style={({ pressed }) => [styles.link, pressed && { opacity: 0.7 }]}
                accessibilityLabel={`Open ${l.label}`}
              >
                <Feather name="link" size={14} color={colors.accent} />
                <Text style={styles.linkText}>{l.label}</Text>
              </Pressable>
            ))}
          </View>
        )}
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
    </View>
  );
}

// The buttons for each friendship state.
function FriendButtons({
  profile,
  onChange,
  onCall,
}: {
  profile: UserProfile;
  onChange: (fn: () => Promise<{ friend_status: string }>) => void;
  onCall: (video: boolean) => void;
}) {
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
          <Button
            title="Unfriend"
            variant="ghost"
            onPress={() =>
              confirmAction('Remove friend?', profile.display_name, 'Remove', () => onChange(() => api.removeFriend(profile.id)))
            }
          />
        </>
      );
    default:
      return null;
  }
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    header: { alignItems: 'center', padding: spacing.xl, gap: spacing.xs },
    overCover: { marginTop: -40, padding: 4, borderRadius: 44, backgroundColor: colors.bg },
    name: { fontSize: 22, fontWeight: '800', color: colors.text, marginTop: spacing.sm },
    username: { color: colors.muted },
    bio: { alignSelf: 'stretch', marginTop: spacing.sm, paddingHorizontal: spacing.lg },
    chips: { flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', gap: spacing.sm, marginTop: spacing.md },
    chip: { paddingHorizontal: 12, paddingVertical: 6, borderRadius: radius.pill, backgroundColor: colors.accentSoft },
    chipText: { color: colors.accent, fontWeight: '600', fontSize: 13 },
    link: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: 6,
      paddingHorizontal: 12,
      paddingVertical: 6,
      borderRadius: radius.pill,
      borderWidth: 1,
      borderColor: colors.border,
      backgroundColor: colors.card,
    },
    linkText: { color: colors.accent, fontWeight: '600', fontSize: 13 },
    stats: { color: colors.muted, marginTop: spacing.md },
    actions: { flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'center', gap: spacing.sm, marginTop: spacing.lg },
    section: { alignSelf: 'flex-start', fontWeight: '700', color: colors.text, fontSize: 16, marginTop: spacing.xl },
  });
