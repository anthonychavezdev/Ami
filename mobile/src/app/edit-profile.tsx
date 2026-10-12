import * as ImagePicker from 'expo-image-picker';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ActivityIndicator, KeyboardAvoidingView, Platform, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { Avatar, Button, ErrorBanner, Input } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { useStyles, useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { radius, spacing, type ThemeColors } from '@/theme';

const USERNAME = /^[A-Za-z0-9_]{3,24}$/;

export default function EditProfileScreen() {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const router = useRouter();
  const { user, update } = useAuth();
  const [username, setUsername] = useState(user?.username ?? '');
  const [displayName, setDisplayName] = useState(user?.display_name ?? '');
  const [pronouns, setPronouns] = useState(user?.pronouns ?? '');
  const [bio, setBio] = useState(user?.bio ?? '');
  const [avatarUrl, setAvatarUrl] = useState(user?.avatar_url ?? '');
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (!user) return null;

  const usernameOk = USERNAME.test(username.trim());
  const changed =
    username.trim() !== user.username || displayName.trim() !== user.display_name || pronouns.trim() !== (user.pronouns ?? '') || bio.trim() !== user.bio || avatarUrl !== (user.avatar_url ?? '');

  // Pictures only; no cropping so animated GIFs stay animated.
  const pickPicture = async () => {
    const result = await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], quality: 1 });
    if (result.canceled) return;
    const asset = result.assets[0];
    setUploading(true);
    setError(null);
    try {
      const att = await api.uploadMedia({ uri: asset.uri, name: asset.fileName ?? 'photo.jpg', type: asset.mimeType ?? 'image/jpeg' });
      if (att.type !== 'image') throw new Error('Profile pictures must be pictures');
      setAvatarUrl(att.url);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setUploading(false);
    }
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const patch: Parameters<typeof api.updateMe>[0] = {};
      if (username.trim() !== user.username) patch.username = username.trim();
      if (displayName.trim() !== user.display_name) patch.display_name = displayName.trim();
      if (pronouns.trim() !== (user.pronouns ?? '')) patch.pronouns = pronouns.trim();
      if (bio.trim() !== user.bio) patch.bio = bio.trim();
      if (avatarUrl !== (user.avatar_url ?? '')) patch.avatar_url = avatarUrl;
      const me = await api.updateMe(patch);
      update(me);
      toast.success('Profile updated');
      router.back();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const preview = { ...user, avatar_url: avatarUrl, display_name: displayName || user.display_name };
  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={{ flex: 1 }} keyboardVerticalOffset={90}>
      <ScrollView contentContainerStyle={styles.container} keyboardShouldPersistTaps="handled">
        <Pressable onPress={() => void pickPicture()} disabled={uploading} style={styles.avatarWrap} accessibilityLabel="Change profile picture">
          <Avatar user={preview} size={96} status={false} />
          <View style={styles.avatarBadge}>{uploading ? <ActivityIndicator color="#fff" size="small" /> : <Text style={styles.avatarBadgeText}>Edit</Text>}</View>
        </Pressable>
        <View style={styles.avatarActions}>
          <Button title={avatarUrl ? 'Change picture' : 'Add picture'} icon="image" variant="secondary" small onPress={() => void pickPicture()} disabled={uploading} />
          {!!avatarUrl && <Button title="Remove" icon="trash-2" variant="ghost" small onPress={() => setAvatarUrl('')} disabled={uploading} />}
        </View>
        <Text style={styles.hint}>JPG, PNG, WebP or GIF (animated GIFs play).</Text>

        <ErrorBanner error={error} onDismiss={() => setError(null)} />

        <Text style={styles.label}>Display name</Text>
        <Input value={displayName} onChangeText={setDisplayName} placeholder="Your name" maxLength={50} />

        <Text style={styles.label}>Username</Text>
        <Input value={username} onChangeText={setUsername} placeholder="username" autoCapitalize="none" autoCorrect={false} maxLength={24} />
        <Text style={[styles.hint, !usernameOk && username.trim() !== user.username && { color: colors.danger }]}>3-24 letters, digits or underscores. Changing it frees your old name for others.</Text>

        <Text style={styles.label}>Pronouns</Text>
        <Input value={pronouns} onChangeText={setPronouns} placeholder="e.g. she/her, he/him, they/them" maxLength={30} />

        <Text style={styles.label}>Bio</Text>
        <Input value={bio} onChangeText={setBio} placeholder="A bit about you" multiline maxLength={500} style={styles.bio} />
        <Text style={styles.hint}>{bio.length}/500</Text>

        <Button title="Save" icon="check" onPress={() => void save()} loading={saving} disabled={!changed || !usernameOk || !displayName.trim() || uploading} style={{ marginTop: spacing.lg }} />
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  container: { padding: spacing.xl, gap: spacing.sm, alignItems: 'stretch' },
  avatarWrap: { alignSelf: 'center' },
  avatarBadge: { position: 'absolute', right: -4, bottom: -4, backgroundColor: colors.accent, borderRadius: radius.pill, paddingHorizontal: 10, paddingVertical: 4, borderWidth: 2, borderColor: colors.bg },
  avatarBadgeText: { color: '#fff', fontSize: 12, fontWeight: '700' },
  avatarActions: { flexDirection: 'row', justifyContent: 'center', gap: spacing.sm, marginTop: spacing.md },
  label: { color: colors.muted, fontWeight: '700', fontSize: 13, textTransform: 'uppercase', marginTop: spacing.md },
  hint: { color: colors.muted, fontSize: 12, textAlign: 'center' },
  bio: { minHeight: 90, textAlignVertical: 'top' },
});
