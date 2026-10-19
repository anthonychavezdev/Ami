import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import { formatBytes } from '@/lib/bytes';
import { useStyles } from '@/lib/theme';
import type { UploadProgress } from '@/lib/types';
import { spacing, type ThemeColors } from '@/theme';

// "Uploading video · 1.2 GB of 3.0 GB" over a bar; the size appears once the
// OS has reported the first bytes.
export function UploadBar({ label, progress }: { label: string; progress: UploadProgress }) {
  const styles = useStyles(makeStyles);
  const percent = progress.total > 0 ? Math.min(100, Math.floor((progress.sent / progress.total) * 100)) : 0;
  const size = progress.total > 0 ? ` · ${formatBytes(progress.sent)} of ${formatBytes(progress.total)}` : '';
  return (
    <View
      style={styles.upload}
      accessibilityRole="progressbar"
      accessibilityLabel={label}
      accessibilityValue={{ min: 0, max: 100, now: percent }}
    >
      <View style={styles.row}>
        <Text numberOfLines={1} style={styles.text}>
          {label}
          {size}
        </Text>
        <Text style={styles.percent}>{percent}%</Text>
      </View>
      <View style={styles.track}>
        <View style={[styles.fill, { width: `${percent}%` }]} />
      </View>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    upload: { flexBasis: '100%', width: '100%', gap: 4 },
    row: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
    text: { color: colors.muted, fontSize: 13, flex: 1 },
    percent: { color: colors.muted, fontSize: 13, fontVariant: ['tabular-nums'] },
    track: { height: 4, borderRadius: 2, backgroundColor: colors.border, overflow: 'hidden' },
    fill: { height: '100%', backgroundColor: colors.accent },
  });
