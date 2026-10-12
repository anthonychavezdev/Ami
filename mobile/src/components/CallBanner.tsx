import { Feather } from '@expo/vector-icons';
import React from 'react';
import { Pressable, StyleSheet, Text } from 'react-native';

import { useStyles } from '@/lib/theme';
import { spacing, type ThemeColors } from '@/theme';

// "Call in progress" strip at the top of a group chat.
export function CallBanner({ inThisCall, others, onPress }: { inThisCall: boolean; others: number; onPress: () => void }) {
  const styles = useStyles(makeStyles);
  if (!inThisCall && others === 0) return null;
  return (
    <Pressable onPress={onPress} style={styles.banner} accessibilityLabel={inThisCall ? 'Return to call' : 'Join call'}>
      <Feather name="phone-call" size={16} color="#fff" />
      <Text style={styles.text}>{inThisCall ? 'You are in this call' : `Call in progress · ${others} in the call`}</Text>
      <Text style={styles.action}>{inThisCall ? 'Return' : 'Join'}</Text>
    </Pressable>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  banner: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, paddingHorizontal: spacing.lg, paddingVertical: spacing.sm, backgroundColor: colors.success },
  text: { flex: 1, color: '#fff', fontWeight: '600' },
  action: { color: '#fff', fontWeight: '800' },
});
