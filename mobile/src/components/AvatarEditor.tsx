import { Feather } from '@expo/vector-icons';
import { Image } from 'expo-image';
import React, { useMemo, useRef, useState } from 'react';
import { PanResponder, Pressable, StyleSheet, Text, View, useWindowDimensions } from 'react-native';

import { useStyles, useTheme } from '@/lib/theme';
import type { AvatarCrop } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

const MAX_BOX = 320;

// Largest centred square of a picture with the given width/height ratio.
export function defaultCrop(aspect: number): AvatarCrop {
  const w = Math.min(1, 1 / aspect); // fraction of the width; the height fraction is w * aspect
  return { x: (1 - w) / 2, y: (1 - w * aspect) / 2, w, aspect };
}

function clamp(crop: AvatarCrop): AvatarCrop {
  const wMax = Math.min(1, 1 / crop.aspect);
  const w = Math.min(wMax, Math.max(0.25 * wMax, crop.w));
  const h = w * crop.aspect;
  return { ...crop, w, x: Math.min(1 - w, Math.max(0, crop.x)), y: Math.min(1 - h, Math.max(0, crop.y)) };
}

// The full picture, dimmed, with a draggable circle showing the part that
// will be visible; +/- change how much of the picture the circle covers.
export function AvatarEditor({ uri, crop, onChange }: { uri: string; crop: AvatarCrop; onChange: (c: AvatarCrop) => void }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const { width: screen } = useWindowDimensions();
  const boxW = Math.min(screen - spacing.xl * 2, MAX_BOX, MAX_BOX * crop.aspect);
  const boxH = boxW / crop.aspect;
  const d = crop.w * boxW;
  const left = crop.x * boxW;
  const top = crop.y * boxH;

  const latest = useRef(crop);
  latest.current = crop;
  const start = useRef(crop);
  const [dragging, setDragging] = useState(false);
  const pan = useMemo(
    () =>
      PanResponder.create({
        onStartShouldSetPanResponder: () => true,
        onMoveShouldSetPanResponder: () => true,
        onPanResponderGrant: () => {
          start.current = latest.current;
          setDragging(true);
        },
        onPanResponderMove: (_, g) => onChange(clamp({ ...start.current, x: start.current.x + g.dx / boxW, y: start.current.y + g.dy / boxH })),
        onPanResponderRelease: () => setDragging(false),
        onPanResponderTerminate: () => setDragging(false),
      }),
    [boxW, boxH, onChange],
  );
  const zoom = (factor: number) => {
    const w = crop.w * factor;
    const cx = crop.x + crop.w / 2;
    const cy = crop.y + (crop.w * crop.aspect) / 2;
    onChange(clamp({ ...crop, w, x: cx - w / 2, y: cy - (w * crop.aspect) / 2 }));
  };

  return (
    <View style={styles.wrap}>
      <View style={[styles.box, { width: boxW, height: boxH }]} {...pan.panHandlers} accessibilityLabel="Picture position: drag the circle">
        <Image source={{ uri }} style={{ width: boxW, height: boxH, opacity: 0.35 }} contentFit="fill" />
        <View pointerEvents="none" style={[styles.circle, { left, top, width: d, height: d, borderRadius: d / 2, borderColor: dragging ? colors.accent : '#fff' }]}>
          <Image source={{ uri }} style={{ position: 'absolute', left: -left, top: -top, width: boxW, height: boxH }} contentFit="fill" />
        </View>
      </View>
      <View style={styles.tools}>
        <Pressable onPress={() => zoom(1 / 0.85)} hitSlop={8} style={styles.tool} accessibilityLabel="Zoom out">
          <Feather name="zoom-out" size={18} color={colors.text} />
        </Pressable>
        <Text style={styles.hint}>Drag the circle to choose what others see</Text>
        <Pressable onPress={() => zoom(0.85)} hitSlop={8} style={styles.tool} accessibilityLabel="Zoom in">
          <Feather name="zoom-in" size={18} color={colors.text} />
        </Pressable>
      </View>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  wrap: { alignItems: 'center', gap: spacing.sm },
  box: { backgroundColor: '#000', borderRadius: radius.md, overflow: 'hidden' },
  circle: { position: 'absolute', overflow: 'hidden', borderWidth: 2 },
  tools: { flexDirection: 'row', alignItems: 'center', gap: spacing.md },
  tool: { width: 36, height: 36, borderRadius: 18, backgroundColor: colors.card, borderWidth: 1, borderColor: colors.border, alignItems: 'center', justifyContent: 'center' },
  hint: { color: colors.muted, fontSize: 12, flexShrink: 1, textAlign: 'center' },
});
