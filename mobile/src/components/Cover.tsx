import { Image } from 'expo-image';
import React, { useState } from 'react';
import { type StyleProp, View, type ViewStyle } from 'react-native';

import { cropStyle } from './ui';
import { API_URL } from '@/lib/config';
import { useTheme } from '@/lib/theme';
import type { Crop } from '@/lib/types';

// A cover is shown three times as wide as it is tall.
export const COVER_RATIO = 3;

// The wide picture across the top of a profile, positioned so its chosen
// window fills the box.
export function Cover({ url, crop, style }: { url: string; crop?: Crop; style?: StyleProp<ViewStyle> }) {
  const { colors } = useTheme();
  const [width, setWidth] = useState(0);
  const height = Math.round(width / COVER_RATIO);
  return (
    <View
      style={[{ width: '100%', aspectRatio: COVER_RATIO, overflow: 'hidden', backgroundColor: colors.border }, style]}
      onLayout={(e) => setWidth(e.nativeEvent.layout.width)}
    >
      {width > 0 && (
        <Image
          source={{ uri: API_URL + url }}
          style={cropStyle(crop, width, height)}
          contentFit={crop?.w ? 'fill' : 'cover'}
          accessibilityLabel="Cover picture"
        />
      )}
    </View>
  );
}
