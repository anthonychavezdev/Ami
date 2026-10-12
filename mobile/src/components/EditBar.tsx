import { Feather } from '@expo/vector-icons';
import React from 'react';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { useStyles, useTheme } from '@/lib/theme';
import { radius, spacing, type ThemeColors } from '@/theme';

// The strip above a composer saying what is being edited or replied to.
export function EditBar({
  icon,
  text,
  cancelLabel = 'Cancel',
  onCancel,
}: {
  icon: React.ComponentProps<typeof Feather>['name'];
  text: string;
  cancelLabel?: string;
  onCancel: () => void;
}) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  return (
    <View style={styles.bar}>
      <Feather name={icon} size={14} color={colors.accent} />
      <Text style={styles.text} numberOfLines={1}>
        {text}
      </Text>
      <Pressable onPress={onCancel} hitSlop={8} accessibilityLabel={cancelLabel}>
        <Feather name="x" size={18} color={colors.muted} />
      </Pressable>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    bar: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: spacing.sm,
      paddingHorizontal: spacing.lg,
      paddingVertical: spacing.sm,
      backgroundColor: colors.card,
      borderTopWidth: 1,
      borderTopColor: colors.border,
      borderRadius: radius.sm,
    },
    text: { flex: 1, color: colors.text, fontSize: 13 },
  });
