import React, { useState } from 'react';
import { Alert, StyleSheet, View } from 'react-native';

import { Button, Input } from './ui';
import { useStyles } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

// Generic "write something and submit" box used for posts, comments and chat.
export function Composer({
  placeholder,
  buttonTitle = 'Post',
  onSubmit,
  multiline = true,
  compact = false,
}: {
  placeholder: string;
  buttonTitle?: string;
  onSubmit: (text: string) => Promise<void>;
  multiline?: boolean;
  compact?: boolean;
}) {
  const styles = useStyles(makeStyles);
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    const body = text.trim();
    if (!body || busy) return;
    setBusy(true);
    try {
      await onSubmit(body);
      setText('');
    } catch (e) {
      Alert.alert('Could not send', e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <View style={[styles.wrap, compact ? styles.row : null]}>
      <Input
        value={text}
        onChangeText={setText}
        placeholder={placeholder}
        multiline={multiline}
        style={[compact ? styles.inputRow : styles.input]}
        onSubmitEditing={multiline ? undefined : submit}
        returnKeyType={multiline ? 'default' : 'send'}
      />
      <Button title={buttonTitle} onPress={submit} loading={busy} disabled={!text.trim()} small style={compact ? undefined : styles.btn} />
    </View>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  wrap: { padding: spacing.lg, backgroundColor: colors.card, borderBottomWidth: 1, borderBottomColor: colors.border, gap: spacing.sm },
  row: { flexDirection: 'row', alignItems: 'center', borderBottomWidth: 0, borderTopWidth: 1, borderTopColor: colors.border, padding: spacing.sm },
  input: { minHeight: 70, textAlignVertical: 'top' },
  inputRow: { flex: 1, maxHeight: 120 },
  btn: { alignSelf: 'flex-end' },
});
