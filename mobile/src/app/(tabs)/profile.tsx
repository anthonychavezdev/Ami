import { useRouter } from 'expo-router';
import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { Avatar, Button } from '@/components/ui';
import { useAuth } from '@/lib/auth';
import { API_URL } from '@/lib/config';
import { colors, spacing } from '@/theme';

export default function ProfileScreen() {
  const { user, logout } = useAuth();
  const router = useRouter();
  if (!user) return null;
  return (
    <View style={styles.container}>
      <Avatar user={user} size={88} />
      <Text style={styles.name}>{user.display_name}</Text>
      <Text style={styles.username}>@{user.username}</Text>
      <View style={styles.actions}>
        <Button title="View my profile" variant="secondary" onPress={() => router.push(`/user/${user.id}`)} />
        <Button title="Log out" variant="danger" onPress={() => void logout()} />
      </View>
      <Text style={styles.server}>Server: {API_URL}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, alignItems: 'center', padding: spacing.xl, paddingTop: 48, gap: spacing.xs },
  name: { fontSize: 24, fontWeight: '800', color: colors.text, marginTop: spacing.md },
  username: { color: colors.muted },
  actions: { width: '100%', gap: spacing.md, marginTop: spacing.xl },
  server: { color: colors.muted, fontSize: 12, marginTop: 'auto' },
});
