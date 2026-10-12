import { Feather } from '@expo/vector-icons';
import { VideoView, useVideoPlayer } from 'expo-video';
import React, { useState } from 'react';
import { Image, Linking, Pressable, StyleProp, StyleSheet, Text, View, ViewStyle } from 'react-native';

import { mediaUrl } from '@/lib/api';
import { useStyles, useTheme } from '@/lib/theme';
import type { Attachment } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

// Pictures and videos in a grid (one per row when alone), links as cards.
// Sizes are computed from the measured width: Yoga does not resolve
// aspectRatio for percentage widths inside a wrapping row.
export function AttachmentList({ items }: { items: Attachment[] }) {
  const styles = useStyles(makeStyles);
  const [width, setWidth] = useState(0);
  const media = items.filter((a) => a.type !== 'link');
  const links = items.filter((a) => a.type === 'link');
  const single = media.length === 1;
  const tile = single ? width : (width - spacing.xs) / 2;
  return (
    <View style={styles.wrap} onLayout={(e) => setWidth(e.nativeEvent.layout.width)}>
      {media.length > 0 && width > 0 && (
        <View style={styles.grid}>
          {media.map((a, i) => {
            const aspectRatio = single ? (a.width && a.height ? a.width / a.height : 4 / 3) : 1;
            const style = [styles.media, { width: tile, height: Math.round(tile / aspectRatio) }];
            return a.type === 'video' ? (
              <Video key={`${a.url}-${i}`} att={a} style={style} />
            ) : (
              <Image key={`${a.url}-${i}`} source={{ uri: mediaUrl(a) }} style={style} resizeMode="cover" accessibilityLabel="Picture" />
            );
          })}
        </View>
      )}
      {links.map((a, i) => (
        <LinkCard key={`${a.url}-${i}`} url={a.url} />
      ))}
    </View>
  );
}

function Video({ att, style }: { att: Attachment; style: StyleProp<ViewStyle> }) {
  const player = useVideoPlayer(mediaUrl(att));
  return <VideoView player={player} style={style} contentFit="contain" nativeControls />;
}

function LinkCard({ url }: { url: string }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  let host = url;
  try {
    host = new URL(url).host || url;
  } catch {
    /* keep the raw url */
  }
  return (
    <Pressable onPress={() => Linking.openURL(url).catch(() => {})} style={({ pressed }) => [styles.link, pressed && { opacity: 0.7 }]}>
      <View style={styles.linkHead}>
        <Feather name="link" size={14} color={colors.muted} />
        <Text style={styles.linkHost}>{host}</Text>
      </View>
      <Text style={styles.linkUrl} numberOfLines={2}>
        {url}
      </Text>
    </Pressable>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  wrap: { marginTop: spacing.md, gap: spacing.sm },
  grid: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.xs },
  media: { backgroundColor: colors.bg, borderRadius: radius.md, overflow: 'hidden' },
  link: { backgroundColor: colors.bg, borderWidth: 1, borderColor: colors.border, borderRadius: radius.md, padding: spacing.md, gap: 2 },
  linkHead: { flexDirection: 'row', alignItems: 'center', gap: 6 },
  linkHost: { color: colors.text, fontWeight: '700' },
  linkUrl: { color: colors.accent, fontSize: 13 },
});
