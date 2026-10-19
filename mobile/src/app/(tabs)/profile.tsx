import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, Switch, Text, View } from 'react-native';

import { PasswordForm } from '@/components/PasswordForm';
import { ProfileEditor } from '@/components/ProfileEditor';
import { UserView } from '@/components/UserView';
import { Avatar, Button, PaneHeader, Panes, SectionLabel, Segmented } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { API_URL } from '@/lib/config';
import { errorMessage } from '@/lib/errors';
import { useWide } from '@/lib/layout';
import { useStyles, useTheme, type ThemePreference } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { radius, spacing, type ThemeColors } from '@/theme';

const THEMES: { key: ThemePreference; label: string }[] = [
  { key: 'light', label: 'Light' },
  { key: 'dark', label: 'Dark' },
  { key: 'system', label: 'System' },
];

// The forms the Me tab opens: their own screens, or on a wide window the pane
// beside the settings, named by ?show= (which /edit-profile and
// /change-password redirect to).
const FORMS = { 'edit-profile': 'Edit profile', 'change-password': 'Change password' } as const;
type Form = keyof typeof FORMS;

export default function ProfileScreen() {
  const { user, logout, update } = useAuth();
  const { colors, preference, setPreference } = useTheme();
  const styles = useStyles(makeStyles);
  const router = useRouter();
  const wide = useWide();
  const { show } = useLocalSearchParams<{ show?: string }>();
  const form = wide && show && show in FORMS ? (show as Form) : null;
  const [saving, setSaving] = useState(false);
  const setInvisible = async (invisible: boolean) => {
    setSaving(true);
    try {
      await api.setInvisible(invisible);
      update({ invisible });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setSaving(false);
    }
  };
  const open = (what: Form) => (wide ? router.setParams({ show: what }) : router.push(`/${what}`));
  const close = () => router.setParams({ show: undefined });
  if (!user) return null;

  const settings = (
    <View style={styles.container}>
      <Avatar user={user} size={88} />
      <Text style={styles.name}>{user.display_name}</Text>
      <Text style={styles.username}>
        @{user.username}
        {user.pronouns ? ` · ${user.pronouns}` : ''}
      </Text>
      <View style={styles.actions}>
        <Button title="Edit profile" icon="edit-2" onPress={() => open('edit-profile')} />
        {!wide && <Button title="View my profile" icon="user" variant="secondary" onPress={() => router.push(`/user/${user.id}`)} />}
        <Button title="Change password" icon="lock" variant="secondary" onPress={() => open('change-password')} />
        <Button title="Log out" icon="log-out" variant="danger" onPress={() => void logout()} />
      </View>
      <View style={styles.section}>
        <SectionLabel style={{ paddingHorizontal: spacing.lg }}>Appearance</SectionLabel>
        <Segmented value={preference} onChange={setPreference} options={THEMES} />
      </View>
      <View style={styles.section}>
        <SectionLabel style={{ paddingHorizontal: spacing.lg }}>Presence</SectionLabel>
        <View style={styles.row}>
          <View style={{ flex: 1 }}>
            <Text style={styles.rowTitle}>Appear offline</Text>
            <Text style={styles.rowHint}>Others see you as offline. You still see everyone&apos;s status.</Text>
          </View>
          <Switch
            value={user.invisible}
            disabled={saving}
            onValueChange={(v) => void setInvisible(v)}
            trackColor={{ true: colors.accent }}
            accessibilityLabel="Appear offline"
          />
        </View>
      </View>
      <Text style={styles.server}>Server: {API_URL}</Text>
    </View>
  );
  if (!wide) return settings;
  // Beside the settings: the form being filled in, else my profile as others see it.
  return (
    <Panes list={settings}>
      {form ? (
        <>
          <PaneHeader title={FORMS[form]} onClose={close} />
          {form === 'edit-profile' ? <ProfileEditor onDone={close} /> : <PasswordForm onDone={close} />}
        </>
      ) : (
        <UserView id={user.id} embedded />
      )}
    </Panes>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    container: { flex: 1, alignItems: 'center', padding: spacing.xl, paddingTop: 48, gap: spacing.xs },
    name: { fontSize: 24, fontWeight: '800', color: colors.text, marginTop: spacing.md },
    username: { color: colors.muted },
    actions: { width: '100%', gap: spacing.md, marginTop: spacing.xl },
    section: { width: '100%', marginTop: spacing.xl },
    row: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: spacing.md,
      backgroundColor: colors.card,
      borderWidth: 1,
      borderColor: colors.border,
      borderRadius: radius.md,
      padding: spacing.lg,
      marginTop: spacing.sm,
    },
    rowTitle: { color: colors.text, fontWeight: '600', fontSize: 15 },
    rowHint: { color: colors.muted, fontSize: 13, marginTop: 2 },
    server: { color: colors.muted, fontSize: 12, marginTop: 'auto' },
  });
