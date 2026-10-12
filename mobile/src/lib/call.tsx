import React, { createContext, useContext, useEffect, useSyncExternalStore } from 'react';
import { NativeModules, Platform } from 'react-native';
import { MediaStream, MediaStreamTrack, RTCIceCandidate, RTCPeerConnection, RTCSessionDescription, mediaDevices, switchCamera } from './webrtc';

import { ICE_SERVERS } from './config';
import type { CallGroup, User } from './types';
import { socket, type WSEvent } from './ws';

export type CallStatus = 'idle' | 'incoming' | 'outgoing' | 'connecting' | 'active';

// Someone else in the call and what they send us.
export interface Participant {
  user: User;
  streams: MediaStream[]; // their camera/microphone stream and, while they share, their screen
  screenId: string | null; // id of the stream that is their screen
  connected: boolean;
}

export interface CallState {
  status: CallStatus;
  room: string | null; // the server's room key, once known
  group: CallGroup | null; // a group call
  peer: User | null; // one-to-one: the other person; while ringing: whoever is calling
  participants: Participant[];
  video: boolean; // placed as a video call
  localStream: MediaStream | null;
  localScreen: MediaStream | null;
  muted: boolean;
  cameraOn: boolean;
  sharing: boolean;
  error: string | null;
  version: number; // bumped whenever tracks change so views re-render
}

const IDLE: CallState = {
  status: 'idle',
  room: null,
  group: null,
  peer: null,
  participants: [],
  video: false,
  localStream: null,
  localScreen: null,
  muted: false,
  cameraOn: false,
  sharing: false,
  error: null,
  version: 0,
};

type SDP = { type: string; sdp: string };

// One connection per other participant (full mesh), each run with the
// "perfect negotiation" pattern. Of any two people, the one with the higher
// user id is the polite peer.
class Peer {
  makingOffer = false;
  ignoreOffer = false;
  pending: any[] = []; // ICE candidates that arrived before the remote description
  constructor(
    public pc: RTCPeerConnection,
    public polite: boolean,
  ) {}
}

// Calls are rooms on the server (see server/calls.go): a one-to-one call with a
// friend or a group call any member may join. Whoever joins a room sends an
// offer to everyone already in it; media flows directly between participants.
class CallManager {
  private peers = new Map<number, Peer>();
  private early = new Map<number, any[]>(); // ICE candidates from people we have no connection with yet
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
    socket.on('hello', (e) => {
      this.myId = e.user?.id ?? 0;
    });
    socket.on('call.ring', (e) => this.onRing(e));
    socket.on('call.joined', (e) => this.onJoined(e));
    socket.on('call.peer_joined', (e) => this.onPeerJoined(e));
    socket.on('call.peer_left', (e) => this.onPeerLeft(e));
    socket.on('call.declined', (e) => this.onDeclined(e));
    socket.on('call.ended', (e) => this.onEnded(e));
    socket.on('call.unavailable', (e) => this.onUnavailable(e));
    socket.on('call.offer', (e) => void this.onOffer(e));
    socket.on('call.answer', (e) => void this.onAnswer(e));
    socket.on('call.ice', (e) => void this.onIce(e));
    socket.on('call.streams', (e) => this.onStreams(e));
    socket.on('error', (e) => {
      if (this.state.status !== 'idle') this.end(String(e.error ?? 'Call failed'), false);
    });
    // The server drops us from the room a few seconds after our connection
    // goes; give a reconnect that long before giving up on the call.
    socket.on('close', () => {
      if (this.state.status === 'idle' || this.lostTimer) return;
      this.lostTimer = setTimeout(() => {
        this.lostTimer = null;
        if (this.state.status !== 'idle') this.teardown('Connection lost');
      }, 6000);
    });
    socket.on('open', () => {
      if (this.lostTimer) clearTimeout(this.lostTimer);
      this.lostTimer = null;
      // Reconnected mid-call: keep our seat (the server frees it otherwise) and
      // catch up on who came and went meanwhile (call.joined answers).
      const { status, room } = this.state;
      if (room && (status === 'active' || status === 'connecting' || status === 'outgoing')) socket.send({ type: 'call.join', room });
    });
  }

  // ---- store plumbing
  subscribe = (l: () => void) => {
    this.listeners.add(l);
    return () => void this.listeners.delete(l);
  };
  getState = () => this.state;
  private set(patch: Partial<CallState>) {
    this.state = { ...this.state, ...patch };
    this.listeners.forEach((l) => l());
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
    if (this.state.status !== 'idle') return;
    this.set({ ...IDLE, status: 'outgoing', peer, video, version: this.state.version });
    try {
      const stream = await this.getLocalMedia(video);
      if (this.status() !== 'outgoing') return stopAll(stream); // hung up while the mic was being asked for
      this.set({ localStream: stream, cameraOn: video });
      if (!socket.send({ type: 'call.join', user_id: peer.id, video })) throw new Error('Not connected');
      this.ringTimer = setTimeout(() => {
        if (this.state.status === 'outgoing') this.end('No answer', true);
      }, 45_000);
    } catch (e) {
      this.end(errorText(e), true);
    }
  }

  /** Start a group's call, or join the one in progress. */
  async joinGroupCall(group: CallGroup, video: boolean) {
    const room = `g:${group.id}`;
    if (this.state.room === room && this.state.status !== 'idle' && this.state.status !== 'incoming') return; // already in it
    if (this.state.status !== 'idle' && !(this.state.status === 'incoming' && this.state.room === room)) return;
    this.set({ ...IDLE, status: 'connecting', room, group, video, version: this.state.version });
    try {
      const stream = await this.getLocalMedia(video);
      if (this.status() !== 'connecting') return stopAll(stream);
      this.set({ localStream: stream, cameraOn: video });
      if (!socket.send({ type: 'call.join', group_id: group.id, video })) throw new Error('Not connected');
    } catch (e) {
      this.end(errorText(e), true);
    }
  }

  /** Pick up the call we are being rung for. */
  async accept() {
    if (this.state.status !== 'incoming' || !this.state.room) return;
    const { room, video } = this.state;
    this.set({ status: 'connecting' });
    try {
      const stream = await this.getLocalMedia(video);
      if (this.status() !== 'connecting') return stopAll(stream);
      this.set({ localStream: stream, cameraOn: video });
      if (!socket.send({ type: 'call.join', room })) throw new Error('Not connected');
    } catch (e) {
      this.end(errorText(e), true);
    }
  }

  reject() {
    if (this.state.status !== 'incoming') return;
    this.end(null, true);
  }

  hangup() {
    if (this.state.status === 'idle') return;
    this.end(null, true);
  }

  toggleMute() {
    const muted = !this.state.muted;
    this.state.localStream?.getAudioTracks().forEach((t) => (t.enabled = !muted));
    this.set({ muted });
  }

  async toggleCamera() {
    const stream = this.state.localStream;
    if (!stream) return;
    let track = stream.getVideoTracks()[0];
    if (!track) {
      // Voice call: acquire the camera now and renegotiate with everyone.
      try {
        const cam = await mediaDevices.getUserMedia({ video: { facingMode: this.facing } });
        track = cam.getVideoTracks()[0];
        stream.addTrack(track);
        this.peers.forEach((p) => p.pc.addTrack(track, stream));
        this.set({ cameraOn: true });
        this.bump();
      } catch (e) {
        this.set({ error: errorText(e) });
      }
      return;
    }
    const cameraOn = !this.state.cameraOn;
    track.enabled = cameraOn;
    this.set({ cameraOn });
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
    if (this.state.status === 'idle' || this.state.status === 'incoming') return;
    if (this.state.localScreen) {
      this.stopScreenShare();
      return;
    }
    try {
      if (Platform.OS === 'ios' && this.pickerHandle != null) {
        // iOS requires the ReplayKit picker to be shown by the app; the actual
        // capture happens in the Broadcast Upload Extension (see README).
        NativeModules.ScreenCapturePickerViewManager?.show(this.pickerHandle);
      }
      // On the web, {audio: true} also captures the shared window/tab audio;
      // react-native-webrtc on Android/iOS captures video only.
      const screen: MediaStream = await (mediaDevices.getDisplayMedia as any)({ video: true, audio: true });
      screen.getTracks().forEach((t: MediaStreamTrack) => {
        this.peers.forEach((p) => p.pc.addTrack(t, screen));
        t.onended = () => this.stopScreenShare();
      });
      this.set({ localScreen: screen, sharing: true });
      this.announceStreams();
    } catch (e) {
      this.set({ error: errorText(e) });
    }
  }

  private stopScreenShare() {
    const screen = this.state.localScreen;
    if (!screen) return;
    const ids = new Set(screen.getTracks().map((t) => t.id));
    this.peers.forEach(({ pc }) =>
      pc.getSenders().forEach((s) => {
        if (s.track && ids.has(s.track.id)) pc.removeTrack(s);
      }),
    );
    stopAll(screen);
    this.set({ localScreen: null, sharing: false });
    this.announceStreams();
  }

  clearError() {
    this.set({ error: null });
  }

  // ---- internals
  private async getLocalMedia(video: boolean): Promise<MediaStream> {
    return mediaDevices.getUserMedia({ audio: true, video: video ? { facingMode: this.facing } : false });
  }

  private participant(id: number) {
    return this.state.participants.find((p) => p.user.id === id);
  }

  private addParticipant(user: User) {
    if (this.participant(user.id)) return;
    this.set({ participants: [...this.state.participants, { user, streams: [], screenId: null, connected: false }] });
  }

  private updateParticipant(id: number, fn: (p: Participant) => Participant) {
    this.set({ participants: this.state.participants.map((p) => (p.user.id === id ? fn(p) : p)) });
  }

  /** Opens a connection to a participant. With `offer`, our tracks go in now (which starts negotiation). */
  private createPeer(user: User, offer: boolean): Peer {
    const pc = new RTCPeerConnection({ iceServers: ICE_SERVERS });
    const peer = new Peer(pc, this.myId > user.id);
    this.peers.set(user.id, peer);
    peer.pending = this.early.get(user.id) ?? [];
    this.early.delete(user.id);
    if (offer) this.addLocalTracks(pc);

    pc.onnegotiationneeded = async () => {
      try {
        peer.makingOffer = true;
        await pc.setLocalDescription();
        this.signal(user.id, { type: 'call.offer', sdp: sdpOf(pc.localDescription) });
      } catch (e) {
        console.warn('negotiation failed', e);
      } finally {
        peer.makingOffer = false;
      }
    };

    pc.onicecandidate = (ev: any) => {
      if (ev.candidate) this.signal(user.id, { type: 'call.ice', candidate: ev.candidate.toJSON() });
    };

    pc.ontrack = (ev: any) => {
      const stream: MediaStream | undefined = ev.streams?.[0];
      if (!stream) return;
      this.updateParticipant(user.id, (p) => ({ ...p, streams: p.streams.some((s) => s.id === stream.id) ? p.streams : [...p.streams, stream] }));
      this.bump();
    };

    pc.onconnectionstatechange = () => {
      switch (pc.connectionState) {
        case 'connected':
          if (this.ringTimer) clearTimeout(this.ringTimer);
          this.updateParticipant(user.id, (p) => ({ ...p, connected: true }));
          if (this.state.status !== 'active') this.set({ status: 'active' });
          this.announceStreams(user.id);
          break;
        case 'failed':
        case 'closed':
          if (this.peers.get(user.id) === peer) this.dropPeer(user.id, 'Connection lost');
          break;
      }
    };
    return peer;
  }

  private addLocalTracks(pc: RTCPeerConnection) {
    const { localStream, localScreen } = this.state;
    localStream?.getTracks().forEach((t) => pc.addTrack(t, localStream));
    localScreen?.getTracks().forEach((t) => pc.addTrack(t, localScreen));
  }

  /** Forgets a participant; a one-to-one call is over without the other person. */
  private dropPeer(id: number, reason: string | null) {
    const peer = this.peers.get(id);
    this.peers.delete(id);
    try {
      peer?.pc.close();
    } catch {
      /* already closed */
    }
    this.set({ participants: this.state.participants.filter((p) => p.user.id !== id) });
    if (!this.state.group) this.end(reason, true);
    else this.bump();
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
    if (this.state.status === 'idle') {
      this.set({ ...IDLE, status: 'incoming', room, peer: from_user, group: group ?? null, video: !!video, version: this.state.version });
      this.onIncoming?.();
      return;
    }
    if (room === this.state.room) return;
    if (!group) socket.send({ type: 'call.decline', room, reason: 'busy' }); // a friend calling hears we are busy
  }

  private onJoined(e: WSEvent) {
    const { room, participants, video, group } = e;
    if (this.state.status === 'idle') {
      socket.send({ type: 'call.leave', room }); // hung up before the server answered
      return;
    }
    this.set({ room, video: !!video, group: group ?? this.state.group });
    const others: User[] = participants ?? [];
    // After a reconnect this also answers a re-join: drop whoever left meanwhile,
    // keep the connections we still have, offer to anyone new.
    const present = new Set(others.map((u) => u.id));
    for (const p of this.state.participants) if (!present.has(p.user.id)) this.dropPeer(p.user.id, null);
    if (this.status() === 'idle') return; // a one-to-one call ends without the other person
    const fresh = others.filter((u) => !this.peers.has(u.id));
    if (!others.length) {
      if (this.state.group && this.status() !== 'active') this.set({ status: 'active' }); // alone for now; the call screen says so
      return; // one-to-one: keep ringing until the friend joins
    }
    if (fresh.length && this.status() !== 'active') this.set({ status: 'connecting' });
    for (const u of fresh) {
      this.addParticipant(u);
      this.createPeer(u, true); // our tracks -> negotiationneeded -> offer
    }
  }

  private onPeerJoined(e: WSEvent) {
    if (e.room !== this.state.room || this.state.status === 'idle') return;
    if (e.user.id === this.myId) {
      if (this.state.status === 'incoming') this.teardown(null); // picked up on another of our devices
      return;
    }
    if (this.state.status === 'incoming') return;
    this.addParticipant(e.user); // their offer follows
    if (this.state.status === 'outgoing') {
      if (this.ringTimer) clearTimeout(this.ringTimer);
      this.set({ status: 'connecting' });
    }
  }

  private onPeerLeft(e: WSEvent) {
    if (e.room !== this.state.room || this.state.status === 'idle' || this.state.status === 'incoming' || e.user_id === this.myId) return;
    if (!this.participant(e.user_id)) return;
    this.dropPeer(e.user_id, null);
  }

  private onDeclined(e: WSEvent) {
    if (e.room !== this.state.room || this.state.status === 'idle' || this.state.group) return;
    this.end(e.reason === 'busy' ? 'User is on another call' : 'Call declined', true);
  }

  private onEnded(e: WSEvent) {
    if (e.room !== this.state.room) return;
    if (this.state.status === 'incoming') this.teardown(null); // they hung up before we answered
  }

  private onUnavailable(e: WSEvent) {
    if (e.room !== this.state.room || this.state.status !== 'outgoing') return;
    this.teardown('User is offline');
  }

  // ---- WebRTC signaling
  private async onOffer(e: WSEvent) {
    const { room, from, from_user, sdp } = e;
    if (room !== this.state.room || this.state.status === 'idle' || this.state.status === 'incoming' || !from_user) return;
    let peer = this.peers.get(from);
    const fresh = !peer;
    if (!peer) {
      this.addParticipant(from_user);
      peer = this.createPeer(from_user, false);
    }
    const pc = peer.pc;
    const collision = peer.makingOffer || pc.signalingState !== 'stable';
    peer.ignoreOffer = !peer.polite && collision;
    if (peer.ignoreOffer) return;
    try {
      await pc.setRemoteDescription(new RTCSessionDescription(sdp));
      if (fresh) this.addLocalTracks(pc); // ride on their transceivers in the answer
      await this.flushCandidates(peer);
      await pc.setLocalDescription();
      this.signal(from, { type: 'call.answer', sdp: sdpOf(pc.localDescription) });
    } catch (err) {
      console.warn('offer handling failed', err);
    }
  }

  private async onAnswer(e: WSEvent) {
    const peer = this.peers.get(e.from);
    if (!peer || e.room !== this.state.room || peer.pc.signalingState !== 'have-local-offer') return;
    try {
      await peer.pc.setRemoteDescription(new RTCSessionDescription(e.sdp));
      await this.flushCandidates(peer);
    } catch (err) {
      console.warn('answer handling failed', err);
    }
  }

  private async onIce(e: WSEvent) {
    if (e.room !== this.state.room) return;
    const peer = this.peers.get(e.from);
    if (!peer) {
      this.early.set(e.from, [...(this.early.get(e.from) ?? []), e.candidate]);
      return;
    }
    if (!peer.pc.remoteDescription) {
      peer.pending.push(e.candidate);
      return;
    }
    try {
      await peer.pc.addIceCandidate(new RTCIceCandidate(e.candidate));
    } catch (err) {
      if (!peer.ignoreOffer) console.warn('addIceCandidate failed', err);
    }
  }

  private async flushCandidates(peer: Peer) {
    const list = peer.pending;
    peer.pending = [];
    for (const c of list) {
      try {
        await peer.pc.addIceCandidate(new RTCIceCandidate(c));
      } catch (err) {
        console.warn('addIceCandidate failed', err);
      }
    }
  }

  private onStreams(e: WSEvent) {
    if (e.room !== this.state.room) return;
    const screen: string | null = e.screen ?? null;
    this.updateParticipant(e.from, (p) => ({ ...p, screenId: screen, streams: screen ? p.streams : p.streams.filter((s) => s.id !== p.screenId) }));
    this.bump();
  }

  // ---- ending
  private end(reason: string | null, tellServer: boolean) {

    if (tellServer && this.state.room) socket.send({ type: this.state.status === 'incoming' ? 'call.decline' : 'call.leave', room: this.state.room });
    this.teardown(reason);
  }

  private teardown(reason: string | null) {
    if (__DEV__) console.log('[call] ended', reason ?? 'hung up', 'while', this.state.status);
    if (this.ringTimer) clearTimeout(this.ringTimer);
    this.ringTimer = null;
    const { localStream, localScreen } = this.state;
    if (localStream) stopAll(localStream);
    if (localScreen) stopAll(localScreen);
    this.peers.forEach(({ pc }) => {
      try {
        pc.close();
      } catch {
        /* already closed */
      }
    });
    this.peers.clear();
    this.early.clear();
    this.set({ ...IDLE, error: reason, version: this.state.version + 1 });
  }
}

function stopAll(stream: MediaStream) {
  stream.getTracks().forEach((t) => t.stop());
}

function sdpOf(d: RTCSessionDescription | null): SDP {
  return { type: d?.type ?? 'offer', sdp: d?.sdp ?? '' };
}

function errorText(e: unknown): string {
  if (e instanceof Error) return e.message;
  return String(e);
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
