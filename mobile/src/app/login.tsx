import { Link } from 'expo-router';
import React, { useState } from 'react';
import { KeyboardAvoidingView, Platform, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Button, ErrorBanner, Input } from '@/components/ui';
import { useAuth } from '@/lib/auth';
import { colors, spacing } from '@/theme';

export default function LoginScreen() {
  const { login } = useAuth();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      await login(username.trim(), password);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <SafeAreaView style={styles.safe}>
      <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={styles.container}>
        <Text style={styles.logo}>go-social</Text>
        <Text style={styles.tagline}>Posts, friends, groups, calls.</Text>
        <ErrorBanner error={error} onDismiss={() => setError(null)} />
        <View style={styles.form}>
          <Input placeholder="Username" autoCapitalize="none" autoCorrect={false} value={username} onChangeText={setUsername} />
          <Input placeholder="Password" secureTextEntry value={password} onChangeText={setPassword} onSubmitEditing={submit} />
          <Button title="Log in" onPress={submit} loading={busy} disabled={!username || !password} />
        </View>
        <Link href="/register" style={styles.link}>
          No account? Create one
        </Link>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.bg },
  container: { flex: 1, justifyContent: 'center', padding: spacing.xl, gap: spacing.lg },
  logo: { fontSize: 40, fontWeight: '800', color: colors.accent, textAlign: 'center' },
  tagline: { color: colors.muted, textAlign: 'center', marginBottom: spacing.lg },
  form: { gap: spacing.md },
  link: { color: colors.accent, textAlign: 'center', fontWeight: '600', padding: spacing.md },
});
