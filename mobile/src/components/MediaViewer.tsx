import { Feather } from '@expo/vector-icons';
import { StatusBar } from 'expo-status-bar';
import { VideoView, useVideoPlayer } from 'expo-video';
import React, { useEffect, useState } from 'react';
import { FlatList, Image, Modal, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { mediaUrl } from '@/lib/api';
import type { Attachment } from '@/lib/types';
import { radius, spacing } from '@/theme';

type Size = { width: number; height: number };

// Fullscreen pager for pictures and videos; `index` is the item to show, null hides it.
export function MediaViewer({ items, index, onClose }: { items: Attachment[]; index: number | null; onClose: () => void }) {
  const insets = useSafeAreaInsets();
  const [size, setSize] = useState<Size | null>(null);
  const [current, setCurrent] = useState(0);
  const visible = index !== null;
  useEffect(() => {
    if (index !== null) setCurrent(index);
  }, [index]);

  return (
    <Modal visible={visible} animationType="fade" onRequestClose={onClose} statusBarTranslucent navigationBarTranslucent supportedOrientations={['portrait', 'landscape']}>
      <StatusBar style="light" />
      <View style={styles.backdrop} onLayout={(e) => setSize({ width: e.nativeEvent.layout.width, height: e.nativeEvent.layout.height })}>
        {visible && size && (
          <FlatList
            data={items}
            horizontal
            pagingEnabled
            initialScrollIndex={index}
            getItemLayout={(_, i) => ({ length: size.width, offset: size.width * i, index: i })}
            keyExtractor={(a, i) => `${a.url}-${i}`}
            onMomentumScrollEnd={(e) => setCurrent(Math.round(e.nativeEvent.contentOffset.x / size.width))}
            renderItem={({ item, index: i }) => <Page att={item} size={size} active={i === current} />}
            showsHorizontalScrollIndicator={false}
          />
        )}
        <View style={[styles.bar, { top: insets.top + spacing.sm }]}>
          <Text style={styles.counter}>{items.length > 1 ? `${current + 1} / ${items.length}` : ''}</Text>
          <Pressable onPress={onClose} hitSlop={12} accessibilityLabel="Close" style={({ pressed }) => [styles.close, pressed && { opacity: 0.6 }]}>
            <Feather name="x" size={22} color="#fff" />
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

function Page({ att, size, active }: { att: Attachment; size: Size; active: boolean }) {
  if (att.type === 'video') return <VideoPage att={att} size={size} active={active} />;
  // Pinch to zoom is native on iOS; Android shows the picture fit to the screen.
  return (
    <ScrollView style={size} contentContainerStyle={size} maximumZoomScale={4} minimumZoomScale={1} bouncesZoom centerContent showsVerticalScrollIndicator={false} showsHorizontalScrollIndicator={false}>
      <Image source={{ uri: mediaUrl(att) }} style={size} resizeMode="contain" accessibilityLabel="Fullscreen picture" />
    </ScrollView>
  );
}

function VideoPage({ att, size, active }: { att: Attachment; size: Size; active: boolean }) {
  const player = useVideoPlayer(mediaUrl(att));
  useEffect(() => {
    if (active) player.play();
    else player.pause();
  }, [active, player]);
  return <VideoView player={player} style={size} contentFit="contain" nativeControls accessibilityLabel="Fullscreen video" />;
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, backgroundColor: '#000' },
  bar: { position: 'absolute', left: spacing.lg, right: spacing.lg, flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  counter: { color: '#fff', fontSize: 15, fontWeight: '600', textShadowColor: 'rgba(0,0,0,0.8)', textShadowRadius: 4 },
  close: { width: 40, height: 40, borderRadius: radius.pill, backgroundColor: 'rgba(0,0,0,0.5)', alignItems: 'center', justifyContent: 'center' },
});
