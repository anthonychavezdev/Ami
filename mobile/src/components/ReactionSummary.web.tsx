import React, { useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Pressable, StyleSheet, Text, View } from 'react-native';

import { makePillStyles, reactorNames, sortedReactions, useReactors } from './reactors';
import { useStyles } from '@/lib/theme';
import type { Post } from '@/lib/types';
import { radius, spacing, type ThemeColors } from '@/theme';

// Browser: the reaction pills under a post, with who reacted shown in a tip
// over the pill the pointer rests on. The tip goes through a portal so cards
// and lists never clip it. (ReactionSummary.tsx opens a sheet on hold.)
export function ReactionSummary({ post }: { post: Post }) {
  const pills = useStyles(makePillStyles);
  const [hover, setHover] = useState<{ emoji: string; x: number; y: number } | null>(null);
  const reactions = sortedReactions(post);
  if (reactions.length === 0) return null;
  return (
    <>
      <View style={pills.reactions}>
        {reactions.map(([emoji, count]) => (
          <Pill
            key={emoji}
            label={`${emoji} ${count}`}
            mine={emoji === post.my_reaction}
            onHover={(at) => setHover(at ? { emoji, ...at } : null)}
          />
        ))}
      </View>
      {hover && createPortal(<Tip post={post} emoji={hover.emoji} x={hover.x} y={hover.y} />, document.body)}
    </>
  );
}

function Pill({ label, mine, onHover }: { label: string; mine: boolean; onHover: (at: { x: number; y: number } | null) => void }) {
  const pills = useStyles(makePillStyles);
  const ref = useRef<View>(null);
  return (
    <Pressable
      ref={ref}
      onHoverIn={() => ref.current?.measureInWindow((x, y, w) => onHover({ x: x + w / 2, y }))}
      onHoverOut={() => onHover(null)}
      accessibilityLabel={label}
      style={[pills.pill, mine && pills.mine]}
    >
      <Text style={pills.pillText}>{label}</Text>
    </Pressable>
  );
}

// "👍 Ana García, Luis Martínez and 3 others", centred above the pill once
// its own size is known.
function Tip({ post, emoji, x, y }: { post: Post; emoji: string; x: number; y: number }) {
  const styles = useStyles(makeStyles);
  const [size, setSize] = useState<{ width: number; height: number } | null>(null);
  const { groups, error } = useReactors(post.id, true);
  const users = groups?.find((g) => g.emoji === emoji)?.users;
  const text = error ?? (users ? reactorNames(users) : groups ? 'Nobody' : '…');
  const left = size ? Math.max(4, Math.min(x - size.width / 2, window.innerWidth - size.width - 4)) : x;
  const top = size ? y - size.height - 6 : y;
  return (
    <View style={layer as never}>
      <View
        onLayout={(e) => setSize({ width: e.nativeEvent.layout.width, height: e.nativeEvent.layout.height })}
        style={[styles.tip, { left, top, opacity: size ? 1 : 0 }]}
      >
        <Text style={styles.text}>
          {emoji} {text}
        </Text>
      </View>
    </View>
  );
}

const layer = { position: 'fixed', left: 0, top: 0, right: 0, bottom: 0, zIndex: 1000, pointerEvents: 'none' } as const;

const makeStyles = (colors: ThemeColors) =>
  StyleSheet.create({
    tip: {
      position: 'absolute',
      maxWidth: 320,
      backgroundColor: colors.text,
      borderRadius: radius.md,
      paddingHorizontal: spacing.md,
      paddingVertical: 6,
      shadowColor: '#000',
      shadowOpacity: 0.25,
      shadowRadius: 8,
      shadowOffset: { width: 0, height: 4 },
    },
    text: { color: colors.card, fontSize: 13, fontWeight: '600' },
  });
