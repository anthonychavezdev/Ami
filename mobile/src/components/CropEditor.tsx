import { Feather } from '@expo/vector-icons';
import { Image } from 'expo-image';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { PanResponder, Pressable, StyleSheet, Text, View, useWindowDimensions } from 'react-native';

import { useStyles, useTheme } from '@/lib/theme';
import type { Crop } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

// Largest centred window of the given ratio (width/height; 1 for the round
// profile picture, 3 for the cover) on a picture with the given width/height.
export function defaultCrop(aspect: number, ratio = 1): Crop {
  const w = Math.min(1, ratio / aspect); // fraction of the width; the height fraction is w * aspect / ratio
  return { x: (1 - w) / 2, y: (1 - (w * aspect) / ratio) / 2, w, aspect };
}

// Keeps the window inside the picture and at least a quarter of its largest size.
function clamp(crop: Crop, ratio: number): Crop {
  const wMax = Math.min(1, ratio / crop.aspect);
  const w = Math.min(wMax, Math.max(0.25 * wMax, crop.w));
  const h = (w * crop.aspect) / ratio;
  return { ...crop, w, x: Math.min(1 - w, Math.max(0, crop.x)), y: Math.min(1 - h, Math.max(0, crop.y)) };
}

// The full picture, dimmed, with a draggable window showing the part that
// will be visible: a circle for the profile picture, a wide box for the
// cover. +/- change how much of the picture the window covers.
export function CropEditor({ uri, crop, ratio = 1, onChange }: { uri: string; crop: Crop; ratio?: number; onChange: (c: Crop) => void }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  const { width: screen } = useWindowDimensions();
  const round = ratio === 1;
  const maxBox = round ? 320 : 480;
  const boxW = Math.min(screen - spacing.xl * 2, maxBox, maxBox * crop.aspect);
  const boxH = boxW / crop.aspect;
  const w = crop.w * boxW;
  const h = w / ratio;
  const left = crop.x * boxW;
  const top = crop.y * boxH;

  const latest = useRef(crop); // what the gesture handlers read, since they are created once
  useEffect(() => {
    latest.current = crop;
  });
  const start = useRef(crop);
  const [dragging, setDragging] = useState(false);
  const pan = useMemo(
    () =>
      // eslint-disable-next-line react-hooks/refs -- the handlers run on gestures, not during render
      PanResponder.create({
        onStartShouldSetPanResponder: () => true,
        onMoveShouldSetPanResponder: () => true,
        onPanResponderGrant: () => {
          start.current = latest.current;
          setDragging(true);
        },
        onPanResponderMove: (_, g) =>
          onChange(clamp({ ...start.current, x: start.current.x + g.dx / boxW, y: start.current.y + g.dy / boxH }, ratio)),
        onPanResponderRelease: () => setDragging(false),
        onPanResponderTerminate: () => setDragging(false),
      }),
    [boxW, boxH, ratio, onChange],
  );
  const zoom = (factor: number) => {
    const nw = crop.w * factor;
    const cx = crop.x + crop.w / 2;
    const cy = crop.y + (crop.w * crop.aspect) / ratio / 2;
    onChange(clamp({ ...crop, w: nw, x: cx - nw / 2, y: cy - (nw * crop.aspect) / ratio / 2 }, ratio));
  };

  return (
    <View style={styles.wrap}>
      <View style={[styles.box, { width: boxW, height: boxH }]} {...pan.panHandlers} accessibilityLabel="Picture position: drag the window">
        <Image source={{ uri }} style={{ width: boxW, height: boxH, opacity: 0.35 }} contentFit="fill" />
        <View
          pointerEvents="none"
          style={[
            styles.window,
            { left, top, width: w, height: h, borderRadius: round ? w / 2 : radius.sm, borderColor: dragging ? colors.accent : '#fff' },
          ]}
        >
          <Image source={{ uri }} style={{ position: 'absolute', left: -left, top: -top, width: boxW, height: boxH }} contentFit="fill" />
        </View>
      </View>
      <View style={styles.tools}>
        <Pressable onPress={() => zoom(1 / 0.85)} hitSlop={8} style={styles.tool} accessibilityLabel="Zoom out">
          <Feather name="zoom-out" size={18} color={colors.text} />
        </Pressable>
        <Text style={styles.hint}>{round ? 'Drag the circle to choose what others see' : 'Drag the box to choose what shows'}</Text>
        <Pressable onPress={() => zoom(0.85)} hitSlop={8} style={styles.tool} accessibilityLabel="Zoom in">
          <Feather name="zoom-in" size={18} color={colors.text} />
        </Pressable>
      </View>
    </View>
  );
}

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    wrap: { alignItems: 'center', gap: spacing.sm },
    box: { backgroundColor: '#000', borderRadius: radius.md, overflow: 'hidden' },
    window: { position: 'absolute', overflow: 'hidden', borderWidth: 2 },
    tools: { flexDirection: 'row', alignItems: 'center', gap: spacing.md },
    tool: {
      width: 36,
      height: 36,
      borderRadius: 18,
      backgroundColor: colors.card,
      borderWidth: 1,
      borderColor: colors.border,
      alignItems: 'center',
      justifyContent: 'center',
    },
    hint: { color: colors.muted, fontSize: 12, flexShrink: 1, textAlign: 'center' },
  });
