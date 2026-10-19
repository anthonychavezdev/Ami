import React, { useState } from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { Button, ErrorBanner, Input, KeyboardScreen } from './ui';
import { api } from '@/lib/api';
import { useAuth } from '@/lib/auth';
import { errorMessage } from '@/lib/errors';
import { useStyles } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { spacing, type ThemeColors } from '@/theme';

// Sets a new password: by choice, or because the account was given a
// temporary one. `onDone` is called once it is changed.
export function PasswordForm({ onDone }: { onDone: () => void }) {
  const styles = useStyles(makeStyles);
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
      onDone();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <KeyboardScreen>
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
