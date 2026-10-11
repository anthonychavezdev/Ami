import React, { useRef, useState } from 'react';
import { Modal, Pressable, StyleSheet, Text, View, useWindowDimensions } from 'react-native';

import { DEFAULT_REACTION, REACTION_EMOJIS, colors, radius, spacing } from '@/theme';

// Tap: toggle the default reaction. Press and hold: pick any emoji from a
// popover anchored above the button.
export function ReactionButton({ current, onReact }: { current: string; onReact: (emoji: string | null) => void }) {
  const ref = useRef<View>(null);
  const [anchor, setAnchor] = useState<{ x: number; y: number } | null>(null);
  const { width: screenWidth } = useWindowDimensions();

  const openPicker = () => {
    ref.current?.measureInWindow((x, y) => setAnchor({ x, y }));
  };
  const pick = (emoji: string) => {
    setAnchor(null);
    onReact(emoji === current ? null : emoji);
  };

  const pickerWidth = Math.min(screenWidth - 2 * spacing.lg, 8 * 44 + 2 * spacing.sm);
  const left = anchor ? Math.max(spacing.lg, Math.min(anchor.x, screenWidth - pickerWidth - spacing.lg)) : 0;
  const top = anchor ? anchor.y - 2 * 50 - spacing.md : 0;

  return (
    <>
      <Pressable
        ref={ref}
        onPress={() => onReact(current ? null : DEFAULT_REACTION)}
        onLongPress={openPicker}
        delayLongPress={250}
        style={({ pressed }) => [styles.btn, current ? styles.btnActive : null, pressed && { opacity: 0.7 }]}
      >
        <Text style={styles.btnText}>{current || '👍'}</Text>
        <Text style={[styles.label, current ? styles.labelActive : null]}>{current ? 'Reacted' : 'React'}</Text>
      </Pressable>

      <Modal visible={anchor !== null} transparent animationType="fade" onRequestClose={() => setAnchor(null)}>
        <Pressable style={StyleSheet.absoluteFill} onPress={() => setAnchor(null)}>
          <View style={[styles.picker, { left, top: Math.max(top, 40), width: pickerWidth }]}>
            {REACTION_EMOJIS.map((e) => (
              <Pressable key={e} onPress={() => pick(e)} style={[styles.emoji, e === current && styles.emojiActive]}>
                <Text style={{ fontSize: 26 }}>{e}</Text>
              </Pressable>
            ))}
          </View>
        </Pressable>
      </Modal>
    </>
  );
}

const styles = StyleSheet.create({
  btn: { flexDirection: 'row', alignItems: 'center', gap: 6, paddingVertical: 6, paddingHorizontal: 12, borderRadius: radius.pill },
  btnActive: { backgroundColor: colors.accentSoft },
  btnText: { fontSize: 18 },
  label: { color: colors.muted, fontWeight: '600' },
  labelActive: { color: colors.accent },
  picker: {
    position: 'absolute',
    flexDirection: 'row',
    flexWrap: 'wrap',
    backgroundColor: colors.card,
    borderRadius: radius.lg,
    padding: spacing.sm,
    shadowColor: '#000',
    shadowOpacity: 0.15,
    shadowRadius: 12,
    shadowOffset: { width: 0, height: 4 },
    elevation: 8,
  },
  emoji: { width: 44, height: 50, alignItems: 'center', justifyContent: 'center', borderRadius: radius.sm },
  emojiActive: { backgroundColor: colors.accentSoft },
});
