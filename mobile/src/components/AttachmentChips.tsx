import { Feather } from '@expo/vector-icons';
import React from 'react';
import { Image, Pressable, StyleSheet, Text, View } from 'react-native';

import { mediaUrl } from '@/lib/api';
import { useStyles, useTheme } from '@/lib/theme';
import type { Attachment } from '@/lib/types';
import { UploadBar } from './UploadBar';
import type { Upload } from '@/lib/useAttachments';
import { radius, spacing, type ThemeColors } from '@/theme';

// The pictures, videos and links waiting to go out with a post or message,
// and a progress bar for the one still uploading.
export function AttachmentChips({
  items,
  upload,
  onRemove,
  onCancelUpload,
}: {
  items: Attachment[];
  upload: Upload | null;
  onRemove: (index: number) => void;
  onCancelUpload: () => void;
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
      {upload && (
        <UploadBar
          label={`Uploading ${upload.kind}${upload.waiting > 0 ? ` (${upload.waiting} more waiting)` : ''}`}
          progress={upload}
          onCancel={onCancelUpload}
        />
      )}
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
  });
