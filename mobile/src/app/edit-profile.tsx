import { Image } from 'expo-image';
import * as ImagePicker from 'expo-image-picker';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { AvatarEditor, defaultCrop } from '@/components/AvatarEditor';
import { Avatar, Button, ErrorBanner, Input, KeyboardScreen, SectionLabel } from '@/components/ui';
import { api } from '@/lib/api';
import { API_URL } from '@/lib/config';
import { useAuth } from '@/lib/auth';
import { type ProfileForm, canSave, formFrom, profilePatch, usernameValid } from '@/lib/profileForm';
import { useStyles, useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { radius, spacing, type ThemeColors } from '@/theme';
import { errorMessage } from '@/lib/errors';

export default function EditProfileScreen() {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const router = useRouter();
  const { user, update } = useAuth();
  const [form, setForm] = useState<ProfileForm>(() =>
    user ? formFrom(user) : { username: '', displayName: '', pronouns: '', bio: '', avatarUrl: '', crop: null },
  );
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (!user) return null;

  const set = (patch: Partial<ProfileForm>) => setForm((f) => ({ ...f, ...patch }));

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
      set({ avatarUrl: att.url, crop: att.width && att.height ? defaultCrop(att.width / att.height) : null });
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setUploading(false);
    }
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      update(await api.updateMe(profilePatch(form, user)));
      toast.success('Profile updated');
      router.back();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setSaving(false);
    }
  };

  const preview = {
    ...user,
    avatar_url: form.avatarUrl,
    avatar_crop: form.crop ?? undefined,
    display_name: form.displayName || user.display_name,
  };
  const usernameChanged = form.username.trim() !== user.username;

  return (
    <KeyboardScreen>
      <ScrollView contentContainerStyle={styles.container} keyboardShouldPersistTaps="handled">
        <Pressable
          onPress={() => void pickPicture()}
          disabled={uploading}
          style={styles.avatarWrap}
          accessibilityLabel="Change profile picture"
        >
          <Avatar user={preview} size={96} status={false} />
          <View style={styles.avatarBadge}>
            {uploading ? <ActivityIndicator color="#fff" size="small" /> : <Text style={styles.avatarBadgeText}>Edit</Text>}
          </View>
        </Pressable>
        <View style={styles.avatarActions}>
          <Button
            title={form.avatarUrl ? 'Change picture' : 'Add picture'}
            icon="image"
            variant="secondary"
            small
            onPress={() => void pickPicture()}
            disabled={uploading}
          />
          {!!form.avatarUrl && (
            <Button
              title="Remove"
              icon="trash-2"
              variant="ghost"
              small
              onPress={() => set({ avatarUrl: '', crop: null })}
              disabled={uploading}
            />
          )}
        </View>
        {!!form.avatarUrl &&
          (form.crop ? (
            <AvatarEditor uri={API_URL + form.avatarUrl} crop={form.crop} onChange={(crop) => set({ crop })} />
          ) : (
            // Older pictures carry no size: read it from the picture itself, then the editor appears.
            <Image
              source={{ uri: API_URL + form.avatarUrl }}
              style={{ width: 1, height: 1, opacity: 0 }}
              onLoad={(e) => set({ crop: defaultCrop(e.source.width / e.source.height) })}
            />
          ))}
        <Text style={styles.hint}>JPG, PNG, WebP or GIF (animated GIFs play). The small circle above is what others see.</Text>

        <ErrorBanner error={error} onDismiss={() => setError(null)} />

        <SectionLabel style={{ marginTop: spacing.md }}>Display name</SectionLabel>
        <Input value={form.displayName} onChangeText={(displayName) => set({ displayName })} placeholder="Your name" maxLength={50} />

        <SectionLabel style={{ marginTop: spacing.md }}>Username</SectionLabel>
        <Input
          value={form.username}
          onChangeText={(username) => set({ username })}
          placeholder="username"
          autoCapitalize="none"
          autoCorrect={false}
          maxLength={24}
        />
        <Text style={[styles.hint, !usernameValid(form) && usernameChanged && { color: colors.danger }]}>
          3-24 letters, digits or underscores. Changing it frees your old name for others.
        </Text>

        <SectionLabel style={{ marginTop: spacing.md }}>Pronouns</SectionLabel>
        <Input
          value={form.pronouns}
          onChangeText={(pronouns) => set({ pronouns })}
          placeholder="e.g. she/her, he/him, they/them"
          maxLength={30}
        />

        <SectionLabel style={{ marginTop: spacing.md }}>Bio</SectionLabel>
        <Input
          value={form.bio}
          onChangeText={(bio) => set({ bio })}
          placeholder="A bit about you"
          multiline
          maxLength={500}
          style={styles.bio}
        />
        <Text style={styles.hint}>{form.bio.length}/500</Text>

        <Button
          title="Save"
          icon="check"
          onPress={() => void save()}
          loading={saving}
          disabled={!canSave(form, user) || uploading}
          style={{ marginTop: spacing.lg }}
        />
      </ScrollView>
    </KeyboardScreen>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    container: { padding: spacing.xl, gap: spacing.sm, alignItems: 'stretch' },
    avatarWrap: { alignSelf: 'center' },
    avatarBadge: {
      position: 'absolute',
      right: -4,
      bottom: -4,
      backgroundColor: colors.accent,
      borderRadius: radius.pill,
      paddingHorizontal: 10,
      paddingVertical: 4,
      borderWidth: 2,
      borderColor: colors.bg,
    },
    avatarBadgeText: { color: '#fff', fontSize: 12, fontWeight: '700' },
    avatarActions: { flexDirection: 'row', justifyContent: 'center', gap: spacing.sm, marginTop: spacing.md },
    hint: { color: colors.muted, fontSize: 12, textAlign: 'center' },
    bio: { minHeight: 90, textAlignVertical: 'top' },
  });
