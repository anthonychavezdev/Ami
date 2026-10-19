import { Feather } from '@expo/vector-icons';
import React from 'react';
import { Image, Pressable, StyleSheet, Text, View } from 'react-native';

import { mediaUrl } from '@/lib/api';
import { formatBytes } from '@/lib/bytes';
import { useStyles, useTheme } from '@/lib/theme';
import type { Attachment } from '@/lib/types';
import type { Upload } from '@/lib/useAttachments';
import { radius, spacing, type ThemeColors } from '@/theme';

// The pictures, videos and links waiting to go out with a post or message,
// and a progress bar for the one still uploading.
export function AttachmentChips({
  items,
  upload,
  onRemove,
}: {
  items: Attachment[];
  upload: Upload | null;
  onRemove: (index: number) => void;
}) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  if (items.length === 0 && !upload) return null;
  return (
    <View style={styles.chips}>
      {items.map((a, i) => (
        <View key={`${a.url}-${i}`} style={styles.chip}>
          {a.type === 'image' ? (
            <Image source={{ uri: mediaUrl(a) }} style={styles.thumb} />
          ) : (
            <Feather name={a.type === 'video' ? 'video' : 'link'} size={16} color={colors.muted} style={styles.chipIcon} />
          )}
          <Text numberOfLines={1} style={styles.chipText}>
            {a.type === 'link' ? a.url.replace(/^https?:\/\//, '') : a.type}
          </Text>
          <Pressable onPress={() => onRemove(i)} hitSlop={8} accessibilityLabel="Remove attachment">
            <Feather name="x" size={14} color={colors.muted} />
          </Pressable>
        </View>
      ))}
      {upload && <UploadBar upload={upload} />}
    </View>
  );
}

// "Uploading video · 1.2 GB of 3.0 GB" over a bar; the size appears once the
// OS has reported the first bytes.
function UploadBar({ upload }: { upload: Upload }) {
  const styles = useStyles(makeStyles);
  const percent = upload.total > 0 ? Math.min(100, Math.floor((upload.sent / upload.total) * 100)) : 0;
  const more = upload.waiting > 0 ? ` (${upload.waiting} more waiting)` : '';
  const size = upload.total > 0 ? ` · ${formatBytes(upload.sent)} of ${formatBytes(upload.total)}` : '';
  return (
    <View
      style={styles.upload}
      accessibilityRole="progressbar"
      accessibilityLabel={`Uploading ${upload.kind}`}
      accessibilityValue={{ min: 0, max: 100, now: percent }}
    >
      <View style={styles.uploadRow}>
        <Text numberOfLines={1} style={styles.uploadText}>
          Uploading {upload.kind}
          {more}
          {size}
        </Text>
        <Text style={styles.uploadPercent}>{percent}%</Text>
      </View>
      <View style={styles.track}>
        <View style={[styles.fill, { width: `${percent}%` }]} />
      </View>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    chips: { flexDirection: 'row', flexWrap: 'wrap', alignItems: 'center', gap: spacing.sm },
    chip: {
      flexDirection: 'row',
      alignItems: 'center',
      gap: 6,
      maxWidth: 220,
      paddingVertical: 4,
      paddingLeft: 4,
      paddingRight: 10,
      borderRadius: radius.pill,
      borderWidth: 1,
      borderColor: colors.border,
      backgroundColor: colors.bg,
    },
    thumb: { width: 28, height: 28, borderRadius: 14 },
    chipIcon: { paddingLeft: 6 },
    chipText: { color: colors.text, fontSize: 13, flexShrink: 1 },
    upload: { flexBasis: '100%', gap: 4 },
    uploadRow: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
    uploadText: { color: colors.muted, fontSize: 13, flex: 1 },
    uploadPercent: { color: colors.muted, fontSize: 13, fontVariant: ['tabular-nums'] },
    track: { height: 4, borderRadius: 2, backgroundColor: colors.border, overflow: 'hidden' },
    fill: { height: '100%', backgroundColor: colors.accent },
  });
