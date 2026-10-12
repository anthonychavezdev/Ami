import React, { createContext, useContext, useEffect, useSyncExternalStore } from 'react';
import { NativeModules, Platform } from 'react-native';
import { type MediaStream, type MediaStreamTrack, mediaDevices, switchCamera } from './webrtc';

import { Peer } from './callPeer';
import { IDLE, type CallState, type CallStatus, connected, inCall, participantOf, updateParticipant, withParticipant, withScreen, withStream, withoutParticipant } from './callState';
import type { CallGroup, User } from './types';
import { socket, type WSEvent } from './ws';

export type { CallState, CallStatus, Participant } from './callState';
export { cameraOf, sharerOf } from './callState';

const RING_TIMEOUT = 45_000;
const RECONNECT_GRACE = 6_000; // the server frees our seat a few seconds after our connection drops

// Calls are rooms on the server (see server/calls.go): a one-to-one call with a
// friend or a group call any member may join. Whoever joins a room sends an
// offer to everyone already in it; media flows directly between participants,
// one Peer connection each. State changes go through pure helpers in
// callState.ts; this class only sequences them and talks to the server.
class CallManager {
  private peers = new Map<number, Peer>();
  private early = new Map<number, unknown[]>(); // ICE candidates from people we have no connection with yet
  private state: CallState = IDLE;
  private listeners = new Set<() => void>();
  private myId = 0;
  private facing: 'user' | 'environment' = 'user';
  private ringTimer: ReturnType<typeof setTimeout> | null = null;
  private lostTimer: ReturnType<typeof setTimeout> | null = null;
  /** iOS: native handle of a rendered ScreenCapturePickerView (needed before getDisplayMedia). */
  pickerHandle: number | null = null;
  /** Set by the UI so an incoming call can open the call screen. */
  onIncoming: (() => void) | null = null;

  constructor() {
    socket.on('hello', (e) => void (this.myId = e.user?.id ?? 0));
    socket.on('call.ring', (e) => this.onRing(e));
    socket.on('call.joined', (e) => this.onJoined(e));
    socket.on('call.peer_joined', (e) => this.onPeerJoined(e));
    socket.on('call.peer_left', (e) => this.onPeerLeft(e));
    socket.on('call.declined', (e) => this.onDeclined(e));
    socket.on('call.ended', (e) => this.onEnded(e));
    socket.on('call.unavailable', (e) => this.onUnavailable(e));
    socket.on('call.offer', (e) => void this.onOffer(e));
    socket.on('call.answer', (e) => void this.peerIn(e)?.acceptAnswer(e.sdp));
    socket.on('call.ice', (e) => this.onIce(e));
    socket.on('call.streams', (e) => this.onStreams(e));
    socket.on('error', (e) => this.status() !== 'idle' && this.end(String(e.error ?? 'Call failed'), false));
    socket.on('close', () => this.onSocketClosed());
    socket.on('open', () => this.onSocketOpened());
  }

  // ---- store plumbing
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => void this.listeners.delete(listener);
  };
  getState = () => this.state;
  private set(patch: Partial<CallState>) {
    this.update((state) => ({ ...state, ...patch }));
  }
  private update(change: (state: CallState) => CallState) {
    this.state = change(this.state);
    this.listeners.forEach((listener) => listener());
  }
  private bump() {
    this.set({ version: this.state.version + 1 });
  }
  // The status after an await (TypeScript keeps the narrowing from before it otherwise).
  private status(): CallStatus {
    return this.state.status;
  }

  // ---- public actions

  /** Call a friend. */
  async startCall(peer: User, video: boolean) {
    if (this.status() !== 'idle') return;
    this.set({ ...IDLE, status: 'outgoing', peer, video, version: this.state.version });
    if (await this.joinWith(video, 'outgoing', { user_id: peer.id, video })) {
      this.ringTimer = setTimeout(() => this.status() === 'outgoing' && this.end('No answer', true), RING_TIMEOUT);
    }
  }

  /** Start a group's call, or join the one in progress. */
  async joinGroupCall(group: CallGroup, video: boolean) {
    const room = `g:${group.id}`;
    const ringingForIt = this.status() === 'incoming' && this.state.room === room;
    if (this.status() !== 'idle' && !ringingForIt) return; // already in it, or busy
    this.set({ ...IDLE, status: 'connecting', room, group, video, version: this.state.version });
    await this.joinWith(video, 'connecting', { group_id: group.id, video });
  }

  /** Pick up the call we are being rung for. */
  async accept() {
    if (this.status() !== 'incoming' || !this.state.room) return;
    const { room, video } = this.state;
    this.set({ status: 'connecting' });
    await this.joinWith(video, 'connecting', { room });
  }

  reject() {
    if (this.status() === 'incoming') this.end(null, true);
  }

  hangup() {
    if (this.status() !== 'idle') this.end(null, true);
  }

  toggleMute() {
    const muted = !this.state.muted;
    this.state.localStream?.getAudioTracks().forEach((t) => void (t.enabled = !muted));
    this.set({ muted });
  }

  async toggleCamera() {
    const stream = this.state.localStream;
    if (!stream) return;
    const track = stream.getVideoTracks()[0];
    if (track) {
      track.enabled = !this.state.cameraOn;
      this.set({ cameraOn: track.enabled });
      return;
    }
    await this.addCamera(stream); // a voice call: acquire the camera now and renegotiate with everyone
  }

  async flipCamera() {
    const track = this.state.localStream?.getVideoTracks()[0];
    if (!track) return;
    this.facing = this.facing === 'user' ? 'environment' : 'user';
    try {
      await track.applyConstraints({ facingMode: this.facing });
    } catch {
      switchCamera(track);
    }
    this.bump();
  }

  async toggleScreenShare() {
    if (!inCall(this.status())) return;
    if (this.state.localScreen) this.stopScreenShare();
    else await this.startScreenShare();
  }

  clearError() {
    this.set({ error: null });
  }

  // ---- local media

  /** Gets the microphone (and camera) and sends the join; a hang-up meanwhile is honoured. */
  private async joinWith(video: boolean, expected: CallStatus, join: Record<string, unknown>): Promise<boolean> {
    try {
      const stream = await mediaDevices.getUserMedia({ audio: true, video: video ? { facingMode: this.facing } : false });
      if (this.status() !== expected) {
        stopAll(stream);
        return false;
      }
      this.set({ localStream: stream, cameraOn: video });
      if (!socket.send({ type: 'call.join', ...join })) throw new Error('Not connected');
      return true;
    } catch (e) {
      this.end(errorText(e), true);
      return false;
    }
  }

  private async addCamera(stream: MediaStream) {
    try {
      const cam = await mediaDevices.getUserMedia({ video: { facingMode: this.facing } });
      const track = cam.getVideoTracks()[0];
      stream.addTrack(track);
      this.peers.forEach((p) => p.addTrack(track, stream));
      this.set({ cameraOn: true });
      this.bump();
    } catch (e) {
      this.set({ error: errorText(e) });
    }
  }

  private async startScreenShare() {
    try {
      if (Platform.OS === 'ios' && this.pickerHandle != null) {
        // iOS requires the ReplayKit picker to be shown by the app; the actual
        // capture happens in the Broadcast Upload Extension (see README).
        NativeModules.ScreenCapturePickerViewManager?.show(this.pickerHandle);
      }
      // On the web, {audio: true} also captures the shared window/tab audio;
      // react-native-webrtc on Android/iOS captures video only.
      const screen: MediaStream = await (mediaDevices.getDisplayMedia as any)({ video: true, audio: true });
      this.peers.forEach((p) => p.addStream(screen));
      screen.getTracks().forEach((t: MediaStreamTrack) => void (t.onended = () => this.stopScreenShare()));
      this.set({ localScreen: screen, sharing: true });
      this.announceStreams();
    } catch (e) {
      this.set({ error: errorText(e) });
    }
  }

  private stopScreenShare() {
    const screen = this.state.localScreen;
    if (!screen) return;
    this.peers.forEach((p) => p.removeStream(screen));
    stopAll(screen);
    this.set({ localScreen: null, sharing: false });
    this.announceStreams();
  }

  // ---- peers

  /** Opens a connection to a participant. With `offer`, our tracks go in now, which starts negotiation. */
  private createPeer(user: User, offer: boolean): Peer {
    const peer = new Peer(this.myId > user.id, this.peerEvents(user), this.early.get(user.id));
    this.early.delete(user.id);
    this.peers.set(user.id, peer);
    if (offer) this.sendLocalStreams(peer);
    return peer;
  }

  private peerEvents(user: User) {
    return {
      onOffer: (sdp: unknown) => this.signal(user.id, { type: 'call.offer', sdp }),
      onAnswer: (sdp: unknown) => this.signal(user.id, { type: 'call.answer', sdp }),
      onCandidate: (candidate: unknown) => this.signal(user.id, { type: 'call.ice', candidate }),
      onStream: (stream: MediaStream) => {
        this.update((s) => updateParticipant(s, user.id, withStream(stream)));
        this.bump();
      },
      onConnected: () => this.onPeerConnected(user.id),
      onLost: () => this.peerIs(user.id) && this.dropPeer(user.id, 'Connection lost'),
    };
  }

  private sendLocalStreams(peer: Peer) {
    const { localStream, localScreen } = this.state;
    if (localStream) peer.addStream(localStream);
    if (localScreen) peer.addStream(localScreen);
  }

  private onPeerConnected(id: number) {
    if (this.ringTimer) clearTimeout(this.ringTimer);
    this.update((s) => ({ ...updateParticipant(s, id, connected), status: 'active' }));
    this.announceStreams(id);
  }

  /** Forgets a participant; a one-to-one call is over without the other person. */
  private dropPeer(id: number, reason: string | null) {
    this.peers.get(id)?.close();
    this.peers.delete(id);
    this.update((s) => withoutParticipant(s, id));
    if (!this.state.group) this.end(reason, true);
    else this.bump();
  }

  private peerIs(id: number) {
    return this.peers.has(id);
  }

  /** The connection a signaling event is for, if it belongs to our room. */
  private peerIn(e: WSEvent) {
    return e.room === this.state.room ? this.peers.get(e.from) : undefined;
  }

  private signal(to: number, msg: Record<string, unknown>) {
    if (this.state.room) socket.send({ ...msg, room: this.state.room, to });
  }

  /** Tells participants which of our streams is the camera and which the screen. */
  private announceStreams(to?: number) {
    const msg = { type: 'call.streams', camera: this.state.localStream?.id ?? null, screen: this.state.localScreen?.id ?? null };
    if (to != null) this.signal(to, msg);
    else this.peers.forEach((_, id) => this.signal(id, msg));
  }

  // ---- room events

  private onRing(e: WSEvent) {
    const { room, from_user, video, group } = e;
    if (this.status() === 'idle') {
      this.set({ ...IDLE, status: 'incoming', room, peer: from_user, group: group ?? null, video: !!video, version: this.state.version });
      this.onIncoming?.();
      return;
    }
    if (room !== this.state.room && !group) socket.send({ type: 'call.decline', room, reason: 'busy' }); // a friend calling hears we are busy
  }

  // Answers our join. After a reconnect it also answers a re-join: drop
  // whoever left meanwhile, keep the connections we still have, offer to anyone new.
  private onJoined(e: WSEvent) {
    const { room, participants, video, group } = e;
    if (this.status() === 'idle') {
      socket.send({ type: 'call.leave', room }); // hung up before the server answered
      return;
    }
    this.set({ room, video: !!video, group: group ?? this.state.group });
    const others: User[] = participants ?? [];
    const present = new Set(others.map((u) => u.id));
    this.state.participants.filter((p) => !present.has(p.user.id)).forEach((p) => this.dropPeer(p.user.id, null));
    if (this.status() === 'idle') return; // a one-to-one call ends without the other person
    const fresh = others.filter((u) => !this.peerIs(u.id));
    if (!others.length) {
      if (this.state.group && this.status() !== 'active') this.set({ status: 'active' }); // alone for now; the call screen says so
      return; // one-to-one: keep ringing until the friend joins
    }
    if (fresh.length && this.status() !== 'active') this.set({ status: 'connecting' });
    fresh.forEach((u) => {
      this.update((s) => withParticipant(s, u));
      this.createPeer(u, true); // our tracks -> negotiationneeded -> offer
    });
  }

  private onPeerJoined(e: WSEvent) {
    if (e.room !== this.state.room || this.status() === 'idle') return;
    if (e.user.id === this.myId) {
      if (this.status() === 'incoming') this.teardown(null); // picked up on another of our devices
      return;
    }
    if (this.status() === 'incoming') return;
    this.update((s) => withParticipant(s, e.user)); // their offer follows
    if (this.status() === 'outgoing') {
      if (this.ringTimer) clearTimeout(this.ringTimer);
      this.set({ status: 'connecting' });
    }
  }

  private onPeerLeft(e: WSEvent) {
    if (e.room !== this.state.room || !inCall(this.status()) || e.user_id === this.myId) return;
    if (participantOf(this.state, e.user_id)) this.dropPeer(e.user_id, null);
  }

  private onDeclined(e: WSEvent) {
    if (e.room !== this.state.room || this.status() === 'idle' || this.state.group) return;
    this.end(e.reason === 'busy' ? 'User is on another call' : 'Call declined', true);
  }

  private onEnded(e: WSEvent) {
    if (e.room === this.state.room && this.status() === 'incoming') this.teardown(null); // they hung up before we answered
  }

  private onUnavailable(e: WSEvent) {
    if (e.room === this.state.room && this.status() === 'outgoing') this.teardown('User is offline');
  }

  private onSocketClosed() {
    if (this.status() === 'idle' || this.lostTimer) return;
    this.lostTimer = setTimeout(() => {
      this.lostTimer = null;
      if (this.status() !== 'idle') this.teardown('Connection lost');
    }, RECONNECT_GRACE);
  }

  // Reconnected mid-call: keep our seat (the server frees it otherwise) and
  // catch up on who came and went meanwhile (call.joined answers).
  private onSocketOpened() {
    if (this.lostTimer) clearTimeout(this.lostTimer);
    this.lostTimer = null;
    if (this.state.room && inCall(this.status())) socket.send({ type: 'call.join', room: this.state.room });
  }

  // ---- WebRTC signaling

  private async onOffer(e: WSEvent) {
    const { room, from, from_user, sdp } = e;
    if (room !== this.state.room || !inCall(this.status()) || !from_user) return;
    const known = this.peers.get(from);
    if (known) return known.acceptOffer(sdp);
    this.update((s) => withParticipant(s, from_user));
    const peer = this.createPeer(from_user, false);
    await peer.acceptOffer(sdp, () => this.sendLocalStreams(peer)); // ride on their transceivers in the answer
  }

  private onIce(e: WSEvent) {
    if (e.room !== this.state.room) return;
    const peer = this.peers.get(e.from);
    if (peer) void peer.addCandidate(e.candidate);
    else this.early.set(e.from, [...(this.early.get(e.from) ?? []), e.candidate]);
  }

  private onStreams(e: WSEvent) {
    if (e.room !== this.state.room) return;
    this.update((s) => updateParticipant(s, e.from, withScreen(e.screen ?? null)));
    this.bump();
  }

  // ---- ending

  private end(reason: string | null, tellServer: boolean) {
    if (tellServer && this.state.room) socket.send({ type: this.status() === 'incoming' ? 'call.decline' : 'call.leave', room: this.state.room });
    this.teardown(reason);
  }

  private teardown(reason: string | null) {
    if (__DEV__) console.log('[call] ended', reason ?? 'hung up', 'while', this.state.status);
    if (this.ringTimer) clearTimeout(this.ringTimer);
    this.ringTimer = null;
    const { localStream, localScreen } = this.state;
    if (localStream) stopAll(localStream);
    if (localScreen) stopAll(localScreen);
    this.peers.forEach((p) => p.close());
    this.peers.clear();
    this.early.clear();
    this.set({ ...IDLE, error: reason, version: this.state.version + 1 });
  }
}

function stopAll(stream: MediaStream) {
  stream.getTracks().forEach((t) => t.stop());
}

function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export const callManager = new CallManager();

const CallContext = createContext<CallManager>(callManager);

export function CallProvider({ children, onIncoming }: { children: React.ReactNode; onIncoming: () => void }) {
  useEffect(() => {
    callManager.onIncoming = onIncoming;
    return () => {
      callManager.onIncoming = null;
    };
  }, [onIncoming]);
  return <CallContext.Provider value={callManager}>{children}</CallContext.Provider>;
}

export function useCall(): { state: CallState; call: CallManager } {
  const call = useContext(CallContext);
  const state = useSyncExternalStore(call.subscribe, call.getState, call.getState);
  return { state, call };
}
