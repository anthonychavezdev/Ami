import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, Switch, Text, View } from 'react-native';

import { Avatar, Button, Segmented } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { toast } from '@/lib/toast';
import { API_URL } from '@/lib/config';
import { useStyles, useTheme, type ThemePreference } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

const THEMES: { key: ThemePreference; label: string }[] = [
  { key: 'light', label: 'Light' },
  { key: 'dark', label: 'Dark' },
  { key: 'system', label: 'System' },
];

export default function ProfileScreen() {
  const { user, logout, update } = useAuth();
  const { colors } = useTheme();
  const [saving, setSaving] = useState(false);
  const { preference, setPreference } = useTheme();
  const setInvisible = async (invisible: boolean) => {
    setSaving(true);
    try {
      await api.setInvisible(invisible);
      update({ invisible });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };
  const styles = useStyles(makeStyles);
  const router = useRouter();
  if (!user) return null;
  return (
    <View style={styles.container}>
      <Avatar user={user} size={88} />
      <Text style={styles.name}>{user.display_name}</Text>
      <Text style={styles.username}>@{user.username}</Text>
      <View style={styles.actions}>
        <Button title="View my profile" icon="user" variant="secondary" onPress={() => router.push(`/user/${user.id}`)} />
        <Button title="Log out" icon="log-out" variant="danger" onPress={() => void logout()} />
      </View>
      <View style={styles.section}>
        <Text style={styles.sectionTitle}>Appearance</Text>
        <Segmented value={preference} onChange={setPreference} options={THEMES} />
      </View>
      <View style={styles.section}>
        <Text style={styles.sectionTitle}>Presence</Text>
        <View style={styles.row}>
          <View style={{ flex: 1 }}>
            <Text style={styles.rowTitle}>Appear offline</Text>
            <Text style={styles.rowHint}>Others see you as offline. You still see everyone's status.</Text>
          </View>
          <Switch value={user.invisible} disabled={saving} onValueChange={(v) => void setInvisible(v)} trackColor={{ true: colors.accent }} accessibilityLabel="Appear offline" />
        </View>
      </View>
      <Text style={styles.server}>Server: {API_URL}</Text>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  container: { flex: 1, alignItems: 'center', padding: spacing.xl, paddingTop: 48, gap: spacing.xs },
  name: { fontSize: 24, fontWeight: '800', color: colors.text, marginTop: spacing.md },
  username: { color: colors.muted },
  actions: { width: '100%', gap: spacing.md, marginTop: spacing.xl },
  section: { width: '100%', marginTop: spacing.xl },
  sectionTitle: { color: colors.muted, fontWeight: '700', fontSize: 13, textTransform: 'uppercase', paddingHorizontal: spacing.lg },
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.md, backgroundColor: colors.card, borderWidth: 1, borderColor: colors.border, borderRadius: radius.md, padding: spacing.lg, marginTop: spacing.sm },
  rowTitle: { color: colors.text, fontWeight: '600', fontSize: 15 },
  rowHint: { color: colors.muted, fontSize: 13, marginTop: 2 },
  server: { color: colors.muted, fontSize: 12, marginTop: 'auto' },
});
