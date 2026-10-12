import React, { useState } from 'react';
import { KeyboardAvoidingView, Platform, StyleSheet, Text, View } from 'react-native';

import { Button, ErrorBanner, Input } from '@/components/ui';
import { useAuth } from '@/lib/auth';
import { setupNotifications } from '@/lib/notifications';
import { useStyles } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';
import { errorMessage } from '@/lib/errors';

export default function RegisterScreen() {
  const styles = useStyles(makeStyles);
  const { register } = useAuth();
  const [username, setUsername] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    if (Platform.OS === 'web') void setupNotifications(); // browsers only ask on a click
    setBusy(true);
    setError(null);
    try {
      await register(username.trim(), password, displayName.trim());
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <KeyboardAvoidingView behavior={Platform.OS === 'ios' ? 'padding' : undefined} style={styles.container}>
      <ErrorBanner error={error} onDismiss={() => setError(null)} />
      <View style={styles.form}>
        <Input
          placeholder="Username (3-24 letters, digits, _)"
          autoCapitalize="none"
          autoCorrect={false}
          value={username}
          onChangeText={setUsername}
        />
        <Input placeholder="Display name (optional)" value={displayName} onChangeText={setDisplayName} />
        <Input
          placeholder="Password (8+ characters)"
          secureTextEntry
          value={password}
          onChangeText={setPassword}
          onSubmitEditing={submit}
        />
        <Button title="Create account" onPress={submit} loading={busy} disabled={username.length < 3 || password.length < 8} />
        <Text style={styles.hint}>Usernames are public. Only friends can message and call you.</Text>
      </View>
    </KeyboardAvoidingView>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    container: { flex: 1, justifyContent: 'center', padding: spacing.xl, backgroundColor: colors.bg },
    form: { gap: spacing.md },
    hint: { color: colors.muted, textAlign: 'center', marginTop: spacing.sm },
  });
