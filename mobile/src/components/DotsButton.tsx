import { Feather } from '@expo/vector-icons';
import React from 'react';
import { Pressable, type StyleProp, type ViewStyle } from 'react-native';

import { pressPoint } from './ContextMenu';
import { useTheme } from '@/lib/theme';

// The ⋯ button that opens an item's menu at the pointer (web, shown on hover).
export function DotsButton({
  label,
  size = 18,
  style,
  onOpen,
}: {
  label: string;
  size?: number;
  style?: StyleProp<ViewStyle>;
  onOpen: (x: number, y: number) => void;
}) {
  const { colors } = useTheme();
  return (
    <Pressable
      onPress={(e) => {
        const { x, y } = pressPoint(e);
        onOpen(x, y);
      }}
      hitSlop={8}
      style={style}
      accessibilityLabel={label}
    >
      <Feather name="more-horizontal" size={size} color={colors.muted} />
    </Pressable>
  );
}
