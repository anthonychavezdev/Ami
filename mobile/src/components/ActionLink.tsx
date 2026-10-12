import { Feather } from '@expo/vector-icons';
import React from 'react';
import { Pressable, StyleSheet, Text } from 'react-native';

import { useStyles, useTheme } from '@/lib/theme';
import type { ThemeColors } from '@/theme';

// A small icon + word action under a comment or message (Reply, Edit, Delete).
export function ActionLink({
  icon,
  label,
  text,
  onPress,
}: {
  icon: React.ComponentProps<typeof Feather>['name'];
  label: string;
  text: string;
  onPress: () => void;
}) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  return (
    <Pressable onPress={onPress} hitSlop={6} style={styles.action} accessibilityLabel={label}>
      <Feather name={icon} size={13} color={colors.muted} />
      <Text style={styles.actionText}>{text}</Text>
    </Pressable>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    action: { flexDirection: 'row', alignItems: 'center', gap: 4 },
    actionText: { color: colors.muted, fontSize: 12, fontWeight: '600' },
  });
