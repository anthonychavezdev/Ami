import { Feather } from '@expo/vector-icons';
import { Image } from 'expo-image';
import * as ImagePicker from 'expo-image-picker';
import React, { useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { COVER_RATIO, Cover } from './Cover';
import { CropEditor, defaultCrop } from './CropEditor';
import { Avatar, Button, ErrorBanner, Input, KeyboardScreen, SectionLabel } from './ui';
import { api } from '@/lib/api';
import { API_URL } from '@/lib/config';
import { useAuth } from '@/lib/auth';
import { isHttpUrl } from '@/lib/markdownEdit';
import {
  MAX_INTERESTS,
  MAX_LINKS,
  type ProfileForm,
  canSave,
  formFrom,
  profilePatch,
  usernameValid,
  withInterests,
} from '@/lib/profileForm';
import { useStyles, useTheme } from '@/lib/theme';
import { toast } from '@/lib/toast';
import type { Crop } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';
import { errorMessage } from '@/lib/errors';

type Picture = 'avatar' | 'cover';

// Cover, picture, names, pronouns, bio, interests and links; the edit-profile
// screen, and the pane beside the Me tab on a wide window. `onDone` is called
// after a save.
export function ProfileEditor({ onDone }: { onDone: () => void }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const { user, update } = useAuth();
  const [form, setForm] = useState<ProfileForm>(() =>
    user
      ? formFrom(user)
      : {
          username: '',
          displayName: '',
          pronouns: '',
          bio: '',
          interests: [],
          links: [],
          avatarUrl: '',
          crop: null,
          cover: '',
          coverCrop: null,
        },
  );
  const [uploading, setUploading] = useState<Picture | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [interest, setInterest] = useState('');
  if (!user) return null;

  const set = (patch: Partial<ProfileForm>) => setForm((f) => ({ ...f, ...patch }));

  // Pictures only; no cropping by the picker so animated GIFs stay animated.
  // The crop starts as the centred window once the picture's size is known.
  const pickPicture = async (which: Picture) => {
    const result = await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], quality: 1 });
    if (result.canceled) return;
    const asset = result.assets[0];
    setUploading(which);
    setError(null);
    try {
      const att = await api.uploadMedia({ uri: asset.uri, type: asset.mimeType ?? 'image/jpeg' });
      if (att.type !== 'image') throw new Error('Profile pictures must be pictures');
      const crop = att.width && att.height ? defaultCrop(att.width / att.height, which === 'cover' ? COVER_RATIO : 1) : null;
      set(which === 'cover' ? { cover: att.url, coverCrop: crop } : { avatarUrl: att.url, crop });
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setUploading(null);
    }
  };

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      update(await api.updateMe(profilePatch(form, user)));
      toast.success('Profile updated');
      onDone();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setSaving(false);
    }
  };

  // Typed tags, one or several separated by commas.
  const addInterest = () => {
    const tags = interest.split(',');
    if (!tags.some((t) => t.trim())) return;
    set({ interests: withInterests(form.interests, tags) });
    setInterest('');
  };
  const setLink = (i: number, patch: Partial<{ label: string; url: string }>) =>
    set({ links: form.links.map((l, j) => (j === i ? { ...l, ...patch } : l)) });

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
        <SectionLabel>Cover</SectionLabel>
        {form.cover ? (
          <Cover url={form.cover} crop={form.coverCrop ?? undefined} style={styles.cover} />
        ) : (
          <View style={[styles.cover, styles.noCover]}>
            <Feather name="image" size={22} color={colors.muted} />
            <Text style={styles.hint}>A wide picture across the top of your profile</Text>
          </View>
        )}
        <View style={styles.pictureActions}>
          <Button
            title={form.cover ? 'Change cover' : 'Add cover'}
            icon="image"
            variant="secondary"
            small
            onPress={() => void pickPicture('cover')}
            disabled={uploading !== null}
            loading={uploading === 'cover'}
          />
          {!!form.cover && (
            <Button
              title="Remove"
              icon="trash-2"
              variant="ghost"
              small
              onPress={() => set({ cover: '', coverCrop: null })}
              disabled={uploading !== null}
            />
          )}
        </View>
        <PictureCrop uri={form.cover} crop={form.coverCrop} ratio={COVER_RATIO} onChange={(coverCrop) => set({ coverCrop })} />

        <SectionLabel style={{ marginTop: spacing.md }}>Picture</SectionLabel>
        <Pressable
          onPress={() => void pickPicture('avatar')}
          disabled={uploading !== null}
          style={styles.avatarWrap}
          accessibilityLabel="Change profile picture"
        >
          <Avatar user={preview} size={96} status={false} />
          <View style={styles.avatarBadge}>
            {uploading === 'avatar' ? <ActivityIndicator color="#fff" size="small" /> : <Text style={styles.avatarBadgeText}>Edit</Text>}
          </View>
        </Pressable>
        <View style={styles.pictureActions}>
          <Button
            title={form.avatarUrl ? 'Change picture' : 'Add picture'}
            icon="image"
            variant="secondary"
            small
            onPress={() => void pickPicture('avatar')}
            disabled={uploading !== null}
          />
          {!!form.avatarUrl && (
            <Button
              title="Remove"
              icon="trash-2"
              variant="ghost"
              small
              onPress={() => set({ avatarUrl: '', crop: null })}
              disabled={uploading !== null}
            />
          )}
        </View>
        <PictureCrop uri={form.avatarUrl} crop={form.crop} ratio={1} onChange={(crop) => set({ crop })} />
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
        <Text style={styles.hint}>Markdown works: **bold**, _italic_, [links](https://…). {form.bio.length}/500</Text>

        <SectionLabel style={{ marginTop: spacing.md }}>Interests</SectionLabel>
        {form.interests.length > 0 && (
          <View style={styles.chips}>
            {form.interests.map((tag) => (
              <View key={tag} style={styles.chip}>
                <Text style={styles.chipText}>{tag}</Text>
                <Pressable
                  onPress={() => set({ interests: form.interests.filter((t) => t !== tag) })}
                  hitSlop={8}
                  accessibilityLabel={`Remove ${tag}`}
                >
                  <Feather name="x" size={14} color={colors.accent} />
                </Pressable>
              </View>
            ))}
          </View>
        )}
        <View style={styles.row}>
          <Input
            value={interest}
            onChangeText={setInterest}
            onSubmitEditing={addInterest}
            placeholder="Add an interest (fútbol, café, …)"
            maxLength={30 * 4}
            style={{ flex: 1 }}
          />
          <Button
            title="Add"
            small
            variant="secondary"
            onPress={addInterest}
            disabled={!interest.trim() || form.interests.length >= MAX_INTERESTS}
          />
        </View>
        <Text style={styles.hint}>Up to {MAX_INTERESTS}; people can find you by them.</Text>

        <SectionLabel style={{ marginTop: spacing.md }}>Links</SectionLabel>
        {form.links.map((l, i) => (
          <View key={i} style={styles.row}>
            <Input value={l.label} onChangeText={(label) => setLink(i, { label })} placeholder="Label" maxLength={30} style={{ flex: 2 }} />
            <Input
              value={l.url}
              onChangeText={(url) => setLink(i, { url })}
              placeholder="https://…"
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="url"
              style={[{ flex: 3 }, !!l.url && !isHttpUrl(l.url.trim()) && { borderColor: colors.danger }]}
            />
            <Pressable onPress={() => set({ links: form.links.filter((_, j) => j !== i) })} hitSlop={8} accessibilityLabel="Remove link">
              <Feather name="x" size={18} color={colors.muted} />
            </Pressable>
          </View>
        ))}
        {form.links.length < MAX_LINKS && (
          <Button
            title="Add link"
            icon="link"
            variant="secondary"
            small
            onPress={() => set({ links: [...form.links, { label: '', url: '' }] })}
            style={{ alignSelf: 'flex-start' }}
          />
        )}
        <Text style={styles.hint}>Your other places: a website, Instagram, GitHub… A label is optional.</Text>

        <Button
          title="Save"
          icon="check"
          onPress={() => void save()}
          loading={saving}
          disabled={!canSave(form, user) || uploading !== null}
          style={{ marginTop: spacing.lg }}
        />
      </ScrollView>
    </KeyboardScreen>
  );
}

// The crop editor for a picture, once its size is known. Older pictures carry
// no size, so it is read from the picture itself first.
function PictureCrop({ uri, crop, ratio, onChange }: { uri: string; crop: Crop | null; ratio: number; onChange: (c: Crop) => void }) {
  if (!uri) return null;
  if (crop) return <CropEditor uri={API_URL + uri} crop={crop} ratio={ratio} onChange={onChange} />;
  return (
    <Image
      source={{ uri: API_URL + uri }}
      style={{ width: 1, height: 1, opacity: 0 }}
      onLoad={(e) => onChange(defaultCrop(e.source.width / e.source.height, ratio))}
    />
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    container: { padding: spacing.xl, gap: spacing.sm, alignItems: 'stretch' },
    cover: { borderRadius: radius.md, overflow: 'hidden' },
    noCover: {
      aspectRatio: COVER_RATIO,
      backgroundColor: colors.card,
      borderWidth: 1,
      borderColor: colors.border,
      alignItems: 'center',
      justifyContent: 'center',
      gap: spacing.xs,
    },
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
    pictureActions: { flexDirection: 'row', justifyContent: 'center', gap: spacing.sm, marginTop: spacing.sm },
    hint: { color: colors.muted, fontSize: 12, textAlign: 'center' },
    bio: { minHeight: 90, textAlignVertical: 'top' },
    row: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
    chips: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm },
    chip: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: 6,
      paddingHorizontal: 12,
      paddingVertical: 6,
      borderRadius: radius.pill,
      backgroundColor: colors.accentSoft,
    },
    chipText: { color: colors.accent, fontWeight: '600', fontSize: 13 },
  });
