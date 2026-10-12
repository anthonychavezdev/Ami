import { Stack, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { Button, ErrorBanner, Input, KeyboardScreen } from '@/components/ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { errorMessage } from '@/lib/errors';
import { useStyles } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { spacing, type ThemeColors } from '@/theme';

// Sets a new password: by choice from the Me tab, or because the account was
// given a temporary password (then nothing else works until this is done; the
// server refuses every other request and the root layout shows only this screen).
export default function ChangePasswordScreen() {
  const styles = useStyles(makeStyles);
  const router = useRouter();
  const { user, update } = useAuth();
  const forced = !!user?.must_change_password;
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [again, setAgain] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const mismatch = next.length > 0 && again.length > 0 && next !== again;
  const canSubmit = current.length > 0 && next.length >= 8 && next === again && !busy;

  const submit = async () => {
    if (!canSubmit) return;
    setBusy(true);
    setError(null);
    try {
      update(await api.changePassword(current, next));
      toast.success('Password changed');
      if (forced) router.replace('/');
      else router.back();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <KeyboardScreen>
      <Stack.Screen
        options={{ title: forced ? 'Set a new password' : 'Change password', headerBackVisible: !forced, gestureEnabled: !forced }}
      />
      <View style={styles.form}>
        {forced && <Text style={styles.hint}>You signed in with a temporary password. Choose a new one to continue.</Text>}
        <ErrorBanner error={error} onDismiss={() => setError(null)} />
        <Input
          placeholder={forced ? 'Temporary password' : 'Current password'}
          secureTextEntry
          value={current}
          onChangeText={setCurrent}
          autoFocus
        />
        <Input placeholder="New password (8+ characters)" secureTextEntry value={next} onChangeText={setNext} />
        <Input placeholder="New password again" secureTextEntry value={again} onChangeText={setAgain} onSubmitEditing={submit} />
        {mismatch && <Text style={styles.mismatch}>The passwords do not match.</Text>}
        <Button title={forced ? 'Save and continue' : 'Change password'} onPress={submit} loading={busy} disabled={!canSubmit} />
      </View>
    </KeyboardScreen>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    form: { padding: spacing.xl, gap: spacing.md },
    hint: { color: colors.muted, marginBottom: spacing.sm },
    mismatch: { color: colors.danger, fontSize: 13 },
  });
