import { Feather } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import React, { useEffect, useRef, useState } from 'react';
import { Platform, Pressable, ScrollView, StyleSheet, Text, View, findNodeHandle } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';

import { Avatar, GroupAvatar } from '@/components/ui';
import { useAuth } from '@/lib/auth';
import { type CallState, useCall } from '@/lib/call';
import { type Layout, type Tile, callLayout, callSubtitle, gridColumns, localVideoOf } from '@/lib/callLayout';
import { type MediaStream, RemoteAudio, ScreenCapturePicker, StreamView, canFlipCamera } from '@/lib/webrtc';
import { useStyles } from '@/lib/theme';
import { toast } from '@/lib/toast';
import { radius, spacing, type ThemeColors } from '@/theme';

export default function CallScreen() {
  const styles = useStyles(makeStyles);
  const { state, call } = useCall();
  const { user: me } = useAuth();
  const pickerRef = useRef<any>(null);
  useScreenCapturePicker(pickerRef);
  useLeaveWhenOver(state);

  if (state.status === 'idle' || !me) return <View style={styles.root} />;
  const layout = callLayout(state, me);
  const ringing = state.status === 'incoming';
  const localVideo = localVideoOf(state);

  return (
    <View style={styles.root}>
      <Backdrop layout={layout} state={state} />

      <SafeAreaView style={styles.overlay} pointerEvents="box-none">
        <View style={styles.top}>
          <Text style={styles.name}>{state.group?.name ?? state.peer?.display_name ?? ''}</Text>
          <Text style={styles.subtitle}>{callSubtitle(state)}</Text>
          {state.error && (
            <Pressable onPress={() => call.clearError()}>
              <Text style={styles.error}>{state.error}</Text>
            </Pressable>
          )}
        </View>
        <View style={{ flex: 1 }}>
          <Middle layout={layout} version={state.version} localScreen={state.localScreen} />
        </View>

        {layout.kind !== 'stage' && !ringing && state.participants.length <= 1 && localVideo && (
          <View style={styles.pips} pointerEvents="none">
            <Video stream={localVideo} style={styles.pip} fit="cover" mirror muted version={state.version} zOrder={2} />
          </View>
        )}

        {ringing ? (
          <View style={styles.controls}>
            <Control label="Decline" icon="x" danger onPress={() => call.reject()} />
            <Control label="Accept" icon="check" success onPress={() => void call.accept()} />
          </View>
        ) : (
          <CallControls state={state} />
        )}
      </SafeAreaView>

      {Platform.OS === 'ios' && <ScreenCapturePicker ref={pickerRef} style={{ width: 0, height: 0 }} />}
      <RemoteAudio streams={state.participants.flatMap((p) => p.streams)} />
    </View>
  );
}

// iOS: register the screen-capture picker so the manager can show it.
function useScreenCapturePicker(pickerRef: React.RefObject<any>) {
  const { call } = useCall();
  useEffect(() => {
    if (Platform.OS === 'ios' && pickerRef.current) call.pickerHandle = findNodeHandle(pickerRef.current);
    return () => {
      call.pickerHandle = null;
    };
  }, [call, pickerRef]);
}

// Leaves the screen (once) when the call ends; the reason shows as a banner
// after this modal has slid away, so it is not hidden behind it.
function useLeaveWhenOver(state: CallState) {
  const { call } = useCall();
  const router = useRouter();
  const left = useRef(false);
  useEffect(() => {
    if (state.status !== 'idle' || left.current) return;
    left.current = true;
    const reason = state.error;
    if (reason) call.clearError();
    if (router.canGoBack()) router.back();
    else router.replace('/');
    if (reason) setTimeout(() => toast.show(reason), 400);
  }, [state.status, state.error, call, router]);
}

// Behind everything: one person full screen (one-to-one), or a big avatar.
function Backdrop({ layout, state }: { layout: Layout; state: CallState }) {
  const styles = useStyles(makeStyles);
  if (layout.kind === 'one') {
    if (layout.tile.stream) return <Video stream={layout.tile.stream} style={StyleSheet.absoluteFill} fit="cover" version={state.version} />;
    return (
      <View style={styles.center}>
        <Avatar user={layout.tile.user} size={120} status={false} />
      </View>
    );
  }
  if (layout.kind !== 'portrait') return null;
  return (
    <View style={styles.center}>
      {state.group && state.status !== 'incoming' ? <GroupAvatar name={state.group.name} size={120} /> : state.peer ? <Avatar user={state.peer} size={120} status={false} /> : null}
    </View>
  );
}

// Between the header and the controls: the shared screen with a strip of
// people, or a grid of everyone (group calls).
function Middle({ layout, version, localScreen }: { layout: Layout; version: number; localScreen: MediaStream | null }) {
  const styles = useStyles(makeStyles);
  if (layout.kind === 'stage') {
    return (
      <View style={{ flex: 1 }}>
        <Video stream={layout.stage} style={styles.stage} fit="contain" version={version} muted={layout.stage === localScreen} />
        <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.stripScroll} contentContainerStyle={styles.strip}>
          {layout.tiles.map((t) => (
            <TileView key={t.key} tile={t} width={110} height={140} version={version} />
          ))}
        </ScrollView>
      </View>
    );
  }
  if (layout.kind === 'grid') return <Grid tiles={layout.tiles} version={version} />;
  return null;
}

// Everyone in equal tiles.
function Grid({ tiles, version }: { tiles: Tile[]; version: number }) {
  const [size, setSize] = useState({ width: 0, height: 0 });
  const cols = gridColumns(tiles.length);
  const rows = Math.ceil(tiles.length / cols);
  return (
    <View style={gridStyle} onLayout={(e) => setSize(e.nativeEvent.layout)}>
      {size.width > 0 && tiles.map((t) => <TileView key={t.key} tile={t} width={size.width / cols} height={size.height / rows} version={version} />)}
    </View>
  );
}

function TileView({ tile, width, height, version }: { tile: Tile; width: number; height: number; version: number }) {
  const styles = useStyles(makeStyles);
  return (
    <View style={{ width, height, padding: 3 }}>
      <View style={styles.tile} accessibilityLabel={`${tile.label} tile`}>
        {tile.stream ? (
          <Video stream={tile.stream} style={StyleSheet.absoluteFill} fit="cover" mirror={tile.mirror} muted={tile.mirror} version={version} />
        ) : (
          <View style={styles.tileCenter}>
            <Avatar user={tile.user} size={Math.max(40, Math.min(width, height) * 0.4)} status={false} />
          </View>
        )}
        <Text style={styles.tileLabel} numberOfLines={1}>
          {tile.label}
        </Text>
      </View>
    </View>
  );
}

// Remounts when tracks change so the view picks up a newly added video track.
function Video({ stream, style, fit, mirror, muted, version, zOrder }: { stream: MediaStream; style: any; fit: 'cover' | 'contain'; mirror?: boolean; muted?: boolean; version: number; zOrder?: number }) {
  return <StreamView key={`${stream.id}:${stream.getVideoTracks().length}:${version}`} stream={stream} style={style} fit={fit} mirror={mirror} muted={muted} zOrder={zOrder} />;
}

function CallControls({ state }: { state: CallState }) {
  const styles = useStyles(makeStyles);
  const { call } = useCall();
  return (
    <View style={styles.controls}>
      <Control label={state.muted ? 'Unmute' : 'Mute'} icon={state.muted ? 'mic-off' : 'mic'} active={state.muted} onPress={() => call.toggleMute()} />
      <Control label={state.cameraOn ? 'Camera off' : 'Camera'} icon={state.cameraOn ? 'camera' : 'camera-off'} active={state.cameraOn} onPress={() => void call.toggleCamera()} />
      {canFlipCamera && <Control label="Flip" icon="refresh-cw" disabled={!state.cameraOn} onPress={() => void call.flipCamera()} />}
      <Control label={state.sharing ? 'Stop share' : 'Share screen'} icon="monitor" active={state.sharing} disabled={state.status !== 'active'} onPress={() => void call.toggleScreenShare()} />
      <Control label={state.group ? 'Leave' : 'End'} icon="phone-off" danger onPress={() => call.hangup()} />
    </View>
  );
}

function Control({ label, icon, onPress, active, danger, success, disabled }: { label: string; icon: React.ComponentProps<typeof Feather>['name']; onPress: () => void; active?: boolean; danger?: boolean; success?: boolean; disabled?: boolean }) {
  const styles = useStyles(makeStyles);
  return (
    <Pressable onPress={onPress} disabled={disabled} style={({ pressed }) => [styles.control, pressed && { opacity: 0.7 }, disabled && { opacity: 0.35 }]}>
      <View style={[styles.controlCircle, active && styles.controlActive, danger && styles.controlDanger, success && styles.controlSuccess]}>
        <Feather name={icon} size={22} color="#fff" />
      </View>
      <Text style={styles.controlLabel}>{label}</Text>
    </Pressable>
  );
}

const gridStyle = { flex: 1, flexDirection: 'row', flexWrap: 'wrap', alignContent: 'center' } as const;

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.dark },
  center: { position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, alignItems: 'center', justifyContent: 'center' },
  overlay: { flex: 1, justifyContent: 'space-between' },
  top: { alignItems: 'center', padding: spacing.xl, gap: 4 },
  name: { color: '#fff', fontSize: 26, fontWeight: '800' },
  subtitle: { color: 'rgba(255,255,255,0.7)', fontSize: 15 },
  error: { color: '#ffb4b4', marginTop: spacing.sm },
  pips: { position: 'absolute', right: spacing.lg, bottom: 140, gap: spacing.sm },
  pip: { width: 110, height: 160, borderRadius: radius.md, overflow: 'hidden', backgroundColor: '#222' },
  stage: { flex: 1, backgroundColor: '#000' },
  stripScroll: { flexGrow: 0 }, // a ScrollView grows like flex: 1 by default, which would steal half of the stage
  strip: { paddingHorizontal: spacing.sm, paddingVertical: spacing.sm, alignItems: 'center' },
  tile: { flex: 1, borderRadius: radius.md, overflow: 'hidden', backgroundColor: '#1c1f2a' },
  tileCenter: { flex: 1, alignItems: 'center', justifyContent: 'center' },
  tileLabel: { position: 'absolute', left: 8, bottom: 6, color: '#fff', fontSize: 12, fontWeight: '600', textShadowColor: 'rgba(0,0,0,0.8)', textShadowRadius: 4 },
  controls: { flexDirection: 'row', justifyContent: 'center', gap: spacing.lg, paddingHorizontal: spacing.lg, paddingBottom: spacing.xl },
  control: { alignItems: 'center', gap: 6, width: 64 },
  controlCircle: { width: 56, height: 56, borderRadius: 28, backgroundColor: 'rgba(255,255,255,0.18)', alignItems: 'center', justifyContent: 'center' },
  controlActive: { backgroundColor: colors.accent },
  controlDanger: { backgroundColor: colors.danger },
  controlSuccess: { backgroundColor: colors.success },
  controlLabel: { color: '#fff', fontSize: 11, textAlign: 'center' },
});
