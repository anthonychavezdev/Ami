import { Feather } from '@expo/vector-icons';
import { StatusBar } from 'expo-status-bar';
import { VideoView, useVideoPlayer } from 'expo-video';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { ActivityIndicator, FlatList, Image, Modal, Platform, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { mediaUrl } from '@/lib/api';
import { saveMedia } from '@/lib/save';
import type { Attachment } from '@/lib/types';
import { radius, spacing } from '@/theme';

type Size = { width: number; height: number };
const THUMB = 56;
const STRIP = THUMB + spacing.md * 2;

// Fullscreen pager for pictures and videos: swipe, side arrows, a thumbnail
// strip (when there are several), arrow keys on the web, and Save.
// `index` is the item to show, null hides the viewer. The accent colour comes
// in as a prop on purpose: on react-native-web, subscribing to the theme
// context from here kept the attachment grid next to it from ever measuring
// its width.
export function MediaViewer({ items, index, accent, onClose }: { items: Attachment[]; index: number | null; accent: string; onClose: () => void }) {
  const insets = useSafeAreaInsets();
  const [size, setSize] = useState<Size | null>(null);
  const [current, setCurrent] = useState(0);
  const [notice, setNotice] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const list = useRef<FlatList<Attachment>>(null);
  const strip = useRef<ScrollView>(null);
  const visible = index !== null;
  const many = items.length > 1;
  const listHeight = size ? size.height - (many ? STRIP + insets.bottom : 0) : 0;

  useEffect(() => {
    if (index !== null) setCurrent(index);
  }, [index]);

  const goTo = useCallback(
    (i: number) => {
      if (i < 0 || i >= items.length) return;
      setCurrent(i);
      list.current?.scrollToIndex({ index: i, animated: true });
    },
    [items.length],
  );

  // Keep the active thumbnail in view.
  useEffect(() => {
    if (!size || !many) return;
    strip.current?.scrollTo({ x: Math.max(0, current * (THUMB + spacing.sm) - size.width / 2 + THUMB / 2), animated: true });
  }, [current, size, many]);

  // Web: arrow keys move between items.
  useEffect(() => {
    if (Platform.OS !== 'web' || !visible) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft') goTo(current - 1);
      else if (e.key === 'ArrowRight') goTo(current + 1);
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [visible, current, goTo]);

  // Short status inside the viewer (the app's banners sit underneath the modal).
  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(null), 2500);
    return () => clearTimeout(t);
  }, [notice]);

  const save = async () => {
    const att = items[current];
    if (!att || saving) return;
    setSaving(true);
    try {
      setNotice(await saveMedia(att));
    } catch (e) {
      setNotice(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const isVideo = items[current]?.type === 'video';
  return (
    <Modal visible={visible} animationType="fade" onRequestClose={onClose} statusBarTranslucent navigationBarTranslucent supportedOrientations={['portrait', 'landscape']}>
      <StatusBar style="light" />
      <View style={styles.backdrop} onLayout={(e) => setSize({ width: e.nativeEvent.layout.width, height: e.nativeEvent.layout.height })}>
        {visible && size && (
          <FlatList
            ref={list}
            data={items}
            horizontal
            pagingEnabled
            style={{ height: listHeight, flexGrow: 0 }}
            initialScrollIndex={index}
            getItemLayout={(_, i) => ({ length: size.width, offset: size.width * i, index: i })}
            keyExtractor={(a, i) => `${a.url}-${i}`}
            onMomentumScrollEnd={(e) => setCurrent(Math.round(e.nativeEvent.contentOffset.x / size.width))}
            renderItem={({ item, index: i }) => <Page att={item} size={{ width: size.width, height: listHeight }} active={i === current} />}
            showsHorizontalScrollIndicator={false}
          />
        )}

        <View style={[styles.bar, { top: insets.top + spacing.sm }]}>
          <Text style={styles.counter}>{many ? `${current + 1} / ${items.length}` : ''}</Text>
          <View style={styles.barActions}>
            <Pressable onPress={() => void save()} hitSlop={10} accessibilityLabel={isVideo ? 'Save video' : 'Save picture'} style={({ pressed }) => [styles.round, pressed && { opacity: 0.6 }]}>
              {saving ? <ActivityIndicator color="#fff" /> : <Feather name="download" size={20} color="#fff" />}
            </Pressable>
            <Pressable onPress={onClose} hitSlop={10} accessibilityLabel="Close" style={({ pressed }) => [styles.round, pressed && { opacity: 0.6 }]}>
              <Feather name="x" size={22} color="#fff" />
            </Pressable>
          </View>
        </View>

        {many && current > 0 && (
          <Pressable onPress={() => goTo(current - 1)} hitSlop={10} accessibilityLabel="Previous" style={({ pressed }) => [styles.arrow, { left: spacing.md, top: listHeight / 2 - 22 }, pressed && { opacity: 0.6 }]}>
            <Feather name="chevron-left" size={28} color="#fff" />
          </Pressable>
        )}
        {many && current < items.length - 1 && (
          <Pressable onPress={() => goTo(current + 1)} hitSlop={10} accessibilityLabel="Next" style={({ pressed }) => [styles.arrow, { right: spacing.md, top: listHeight / 2 - 22 }, pressed && { opacity: 0.6 }]}>
            <Feather name="chevron-right" size={28} color="#fff" />
          </Pressable>
        )}

        {notice && (
          <View style={[styles.notice, { bottom: (many ? STRIP : 0) + insets.bottom + spacing.lg }]}>
            <Text style={styles.noticeText}>{notice}</Text>
          </View>
        )}

        {many && (
          <View style={[styles.strip, { height: STRIP + insets.bottom, paddingBottom: insets.bottom }]}>
            <ScrollView ref={strip} horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.stripContent}>
              {items.map((a, i) => (
                <Pressable key={`${a.url}-${i}`} onPress={() => goTo(i)} accessibilityLabel={`${a.type === 'video' ? 'Video' : 'Picture'} ${i + 1}`} style={[styles.thumb, i === current && { borderColor: accent, opacity: 1 }]}>
                  {a.type === 'video' ? (
                    <View style={styles.thumbVideo}>
                      <Feather name="play" size={18} color="#fff" />
                    </View>
                  ) : (
                    <Image source={{ uri: mediaUrl(a) }} style={StyleSheet.absoluteFill} resizeMode="cover" />
                  )}
                </Pressable>
              ))}
            </ScrollView>
          </View>
        )}
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
  barActions: { flexDirection: 'row', gap: spacing.sm },
  counter: { color: '#fff', fontSize: 15, fontWeight: '600', textShadowColor: 'rgba(0,0,0,0.8)', textShadowRadius: 4 },
  round: { width: 40, height: 40, borderRadius: radius.pill, backgroundColor: 'rgba(0,0,0,0.5)', alignItems: 'center', justifyContent: 'center' },
  arrow: { position: 'absolute', width: 44, height: 44, borderRadius: 22, backgroundColor: 'rgba(0,0,0,0.45)', alignItems: 'center', justifyContent: 'center' },
  notice: { position: 'absolute', alignSelf: 'center', backgroundColor: 'rgba(0,0,0,0.75)', paddingHorizontal: spacing.lg, paddingVertical: spacing.sm, borderRadius: radius.pill },
  noticeText: { color: '#fff', fontSize: 14, fontWeight: '600' },
  strip: { position: 'absolute', left: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.6)', paddingTop: spacing.md },
  stripContent: { paddingHorizontal: spacing.lg, gap: spacing.sm, alignItems: 'center' },
  thumb: { width: THUMB, height: THUMB, borderRadius: radius.sm, overflow: 'hidden', borderWidth: 2, borderColor: 'transparent', opacity: 0.6, backgroundColor: '#222' },
  thumbVideo: { flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: '#333' },
});
