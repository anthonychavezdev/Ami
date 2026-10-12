import { useRouter } from 'expo-router';
import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { Avatar, Button, Segmented } from '@/components/ui';
import { useAuth } from '@/lib/auth';
import { API_URL } from '@/lib/config';
import { useStyles, useTheme, type ThemePreference } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

const THEMES: { key: ThemePreference; label: string }[] = [
  { key: 'light', label: 'Light' },
  { key: 'dark', label: 'Dark' },
  { key: 'system', label: 'System' },
];

export default function ProfileScreen() {
  const { user, logout } = useAuth();
  const { preference, setPreference } = useTheme();
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
  server: { color: colors.muted, fontSize: 12, marginTop: 'auto' },
});
