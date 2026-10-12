import { Feather } from '@expo/vector-icons';
import { VideoView, useVideoPlayer } from 'expo-video';
import React, { useState } from 'react';
import { Image, Linking, Pressable, StyleProp, StyleSheet, Text, View, ViewStyle } from 'react-native';

import { MediaViewer } from './MediaViewer';
import { mediaUrl } from '@/lib/api';
import { useStyles, useTheme } from '@/lib/theme';
import type { Attachment } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

// Pictures and videos in a grid (one per row when alone), links as cards.
// Tapping a picture or video opens it fullscreen.
// Sizes are computed from the measured width: Yoga does not resolve
// aspectRatio for percentage widths inside a wrapping row.
export function AttachmentList({ items, style }: { items: Attachment[]; style?: StyleProp<ViewStyle> }) {
  const styles = useStyles(makeStyles);
  const [width, setWidth] = useState(0);
  const [open, setOpen] = useState<number | null>(null);
  const media = items.filter((a) => a.type !== 'link');
  const links = items.filter((a) => a.type === 'link');
  const single = media.length === 1;
  const tile = single ? width : (width - spacing.xs) / 2;
  return (
    <View style={[styles.wrap, style]} onLayout={(e) => setWidth(e.nativeEvent.layout.width)}>
      {media.length > 0 && width > 0 && (
        <View style={styles.grid}>
          {media.map((a, i) => {
            const aspectRatio = single ? (a.width && a.height ? a.width / a.height : 4 / 3) : 1;
            const box = { width: tile, height: Math.round(tile / aspectRatio) };
            return (
              <Pressable key={`${a.url}-${i}`} onPress={() => setOpen(i)} accessibilityRole="button" accessibilityLabel={a.type === 'video' ? 'Video' : 'Picture'} style={({ pressed }) => [styles.media, box, pressed && { opacity: 0.85 }]}>
                {a.type === 'video' ? <Video att={a} /> : <Image source={{ uri: mediaUrl(a) }} style={StyleSheet.absoluteFill} resizeMode="cover" />}
              </Pressable>
            );
          })}
        </View>
      )}
      {links.map((a, i) => (
        <LinkCard key={`${a.url}-${i}`} url={a.url} />
      ))}
      <MediaViewer items={media} index={open} onClose={() => setOpen(null)} />
    </View>
  );
}

// Inline preview: first frame with a play badge; playback happens in the viewer.
function Video({ att }: { att: Attachment }) {
  const styles = useStyles(makeStyles);
  const player = useVideoPlayer(mediaUrl(att));
  return (
    <>
      <VideoView player={player} style={StyleSheet.absoluteFill} contentFit="cover" nativeControls={false} />
      <View style={styles.play}>
        <Feather name="play" size={22} color="#fff" style={{ marginLeft: 3 }} />
      </View>
    </>
  );
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
  media: { backgroundColor: colors.bg, borderRadius: radius.md, overflow: 'hidden', alignItems: 'center', justifyContent: 'center' },
  play: { width: 48, height: 48, borderRadius: 24, backgroundColor: 'rgba(0,0,0,0.55)', alignItems: 'center', justifyContent: 'center' },
  link: { backgroundColor: colors.bg, borderWidth: 1, borderColor: colors.border, borderRadius: radius.md, padding: spacing.md, gap: 2 },
  linkHead: { flexDirection: 'row', alignItems: 'center', gap: 6 },
  linkHost: { color: colors.text, fontWeight: '700' },
  linkUrl: { color: colors.accent, fontSize: 13 },
});
