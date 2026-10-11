import { useRouter } from 'expo-router';
import React, { useEffect, useRef } from 'react';
import { Alert, Pressable, StyleSheet, Text, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { type MediaStream, RTCView } from 'react-native-webrtc';

import { Avatar } from '@/components/ui';
import { useCall } from '@/lib/call';
import { colors, radius, spacing } from '@/theme';

export default function CallScreen() {
  const { state, call } = useCall();
  const router = useRouter();
  const wasActive = useRef(false);

  // Leave the screen when the call ends.
  useEffect(() => {
    if (state.status !== 'idle') {
      wasActive.current = true;
      return;
    }
    if (state.error) {
      const msg = state.error;
      call.clearError();
      Alert.alert('Call ended', msg);
    }
    if (router.canGoBack()) router.back();
    else router.replace('/');
  }, [state.status, state.error, call, router]);

  const peer = state.peer;
  if (!peer) return <View style={styles.root} />;

  const main = state.remoteStreams.find((s) => s.getVideoTracks().length > 0) ?? null;
  const localVideo = state.cameraOn && state.localStream?.getVideoTracks().length ? state.localStream : null;

  const subtitle = {
    incoming: `Incoming ${state.video ? 'video' : 'voice'} call`,
    outgoing: 'Calling…',
    connecting: 'Connecting…',
    active: state.video || state.cameraOn ? 'Video call' : 'Voice call',
    idle: '',
  }[state.status];

  return (
    <View style={styles.root}>
      {main ? (
        <Video stream={main} style={StyleSheet.absoluteFill} fit="cover" version={state.version} />
      ) : (
        <View style={styles.center}>
          <Avatar user={peer} size={120} />
        </View>
      )}

      <SafeAreaView style={styles.overlay} pointerEvents="box-none">
        <View style={styles.top}>
          <Text style={styles.name}>{peer.display_name}</Text>
          <Text style={styles.subtitle}>{subtitle}</Text>
          {state.error && (
            <Pressable onPress={() => call.clearError()}>
              <Text style={styles.error}>{state.error}</Text>
            </Pressable>
          )}
        </View>

        <View style={styles.pips} pointerEvents="none">
          {localVideo && <Video stream={localVideo} style={styles.pip} fit="cover" mirror version={state.version} zOrder={2} />}
        </View>

        {state.status === 'incoming' ? (
          <View style={styles.controls}>
            <Control label="Decline" glyph="✕" danger onPress={() => call.reject()} />
            <Control label="Accept" glyph="✓" success onPress={() => void call.accept()} />
          </View>
        ) : (
          <View style={styles.controls}>
            <Control label={state.muted ? 'Unmute' : 'Mute'} glyph={state.muted ? '🔇' : '🎙️'} active={state.muted} onPress={() => call.toggleMute()} />
            <Control label={state.cameraOn ? 'Camera off' : 'Camera'} glyph="📷" active={state.cameraOn} onPress={() => void call.toggleCamera()} />
            <Control label="Flip" glyph="🔄" disabled={!state.cameraOn} onPress={() => void call.flipCamera()} />
            <Control label="End" glyph="📵" danger onPress={() => call.hangup()} />
          </View>
        )}
      </SafeAreaView>

    </View>
  );
}

function Video({
  stream,
  style,
  fit,
  mirror,
  version,
  zOrder,
}: {
  stream: MediaStream;
  style: any;
  fit: 'cover' | 'contain';
  mirror?: boolean;
  version: number;
  zOrder?: number;
}) {
  // Remount when tracks change so the native view picks up a newly added video track.
  return <RTCView key={`${stream.id}:${stream.getVideoTracks().length}:${version}`} streamURL={stream.toURL()} style={style} objectFit={fit} mirror={mirror} zOrder={zOrder} />;
}

function Control({
  label,
  glyph,
  onPress,
  active,
  danger,
  success,
  disabled,
}: {
  label: string;
  glyph: string;
  onPress: () => void;
  active?: boolean;
  danger?: boolean;
  success?: boolean;
  disabled?: boolean;
}) {
  return (
    <Pressable onPress={onPress} disabled={disabled} style={({ pressed }) => [styles.control, pressed && { opacity: 0.7 }, disabled && { opacity: 0.35 }]}>
      <View style={[styles.controlCircle, active && styles.controlActive, danger && styles.controlDanger, success && styles.controlSuccess]}>
        <Text style={{ fontSize: 22, color: '#fff' }}>{glyph}</Text>
      </View>
      <Text style={styles.controlLabel}>{label}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.dark },
  center: { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, alignItems: 'center', justifyContent: 'center' },
  overlay: { flex: 1, justifyContent: 'space-between' },
  top: { alignItems: 'center', padding: spacing.xl, gap: 4 },
  name: { color: '#fff', fontSize: 26, fontWeight: '800' },
  subtitle: { color: 'rgba(255,255,255,0.7)', fontSize: 15 },
  error: { color: '#ffb4b4', marginTop: spacing.sm },
  pips: { position: 'absolute', right: spacing.lg, bottom: 140, gap: spacing.sm },
  pip: { width: 110, height: 160, borderRadius: radius.md, overflow: 'hidden', backgroundColor: '#222' },
  controls: { flexDirection: 'row', justifyContent: 'center', gap: spacing.lg, paddingHorizontal: spacing.lg, paddingBottom: spacing.xl },
  control: { alignItems: 'center', gap: 6, width: 64 },
  controlCircle: { width: 56, height: 56, borderRadius: 28, backgroundColor: 'rgba(255,255,255,0.18)', alignItems: 'center', justifyContent: 'center' },
  controlActive: { backgroundColor: colors.accent },
  controlDanger: { backgroundColor: colors.danger },
  controlSuccess: { backgroundColor: colors.success },
  controlLabel: { color: '#fff', fontSize: 11, textAlign: 'center' },
});
