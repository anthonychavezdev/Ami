import React, { createContext, useContext, useEffect, useSyncExternalStore } from 'react';
import {
  MediaStream,
  RTCIceCandidate,
  RTCPeerConnection,
  RTCSessionDescription,
  mediaDevices,
} from 'react-native-webrtc';

import { ICE_SERVERS } from './config';
import type { User } from './types';
import { socket, type WSEvent } from './ws';

export type CallStatus = 'idle' | 'incoming' | 'outgoing' | 'connecting' | 'active';

export interface CallState {
  status: CallStatus;
  peer: User | null;
  video: boolean; // call was placed as a video call
  localStream: MediaStream | null;
  remoteStreams: MediaStream[];
  muted: boolean;
  cameraOn: boolean;
  error: string | null;
  version: number; // bumped whenever tracks change so views re-render
}

const IDLE: CallState = {
  status: 'idle',
  peer: null,
  video: false,
  localStream: null,
  remoteStreams: [],
  muted: false,
  cameraOn: false,
  error: null,
  version: 0,
};

type SDP = { type: string; sdp: string };

// One-call-at-a-time WebRTC manager using the "perfect negotiation" pattern,
// signaled over our WebSocket. The callee is the polite peer.
class CallManager {
  private pc: RTCPeerConnection | null = null;
  private state: CallState = IDLE;
  private listeners = new Set<() => void>();
  private makingOffer = false;
  private ignoreOffer = false;
  private polite = false;
  private pendingOffer: SDP | null = null;
  private pendingCandidates: any[] = [];
  private facing: 'user' | 'environment' = 'user';
  private ringTimer: ReturnType<typeof setTimeout> | null = null;
  /** Set by the UI so an incoming call can open the call screen. */
  onIncoming: (() => void) | null = null;

  constructor() {
    socket.on('call.offer', (e) => void this.onOffer(e));
    socket.on('call.answer', (e) => void this.onAnswer(e));
    socket.on('call.ice', (e) => void this.onIce(e));
    socket.on('call.hangup', (e) => this.onRemoteEnd(e, null));
    socket.on('call.reject', (e) => this.onRemoteEnd(e, 'Call declined'));
    socket.on('call.busy', (e) => this.onRemoteEnd(e, 'User is on another call'));
    socket.on('call.unavailable', (e) => this.onRemoteEnd(e, 'User is offline'));
    socket.on('error', (e) => {
      if (this.state.status !== 'idle') this.end(String(e.error ?? 'Call failed'), false);
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

  // ---- public actions
  async startCall(peer: User, video: boolean) {
    if (this.state.status !== 'idle') return;
    this.polite = false;
    this.pendingCandidates = [];
    this.set({ ...IDLE, status: 'outgoing', peer, video });
    try {
      const stream = await this.getLocalMedia(video);
      const pc = this.createPeer(peer.id);
      this.set({ localStream: stream, cameraOn: video });
      stream.getTracks().forEach((t) => pc.addTrack(t, stream)); // -> negotiationneeded -> offer
      this.ringTimer = setTimeout(() => {
        if (this.state.status === 'outgoing') this.end('No answer', true);
      }, 45_000);
    } catch (e) {
      this.end(errorText(e), false);
    }
  }

  async accept() {
    if (this.state.status !== 'incoming' || !this.pendingOffer || !this.state.peer) return;
    const { peer, video } = this.state;
    this.set({ status: 'connecting' });
    try {
      const stream = await this.getLocalMedia(video);
      const pc = this.createPeer(peer.id);
      await pc.setRemoteDescription(new RTCSessionDescription(this.pendingOffer));
      this.pendingOffer = null;
      stream.getTracks().forEach((t) => pc.addTrack(t, stream));
      await this.flushCandidates();
      await pc.setLocalDescription();
      this.signal({ type: 'call.answer', sdp: sdpOf(pc.localDescription) });
      this.set({ localStream: stream, cameraOn: video });
    } catch (e) {
      this.end(errorText(e), true);
    }
  }

  reject() {
    if (this.state.status !== 'incoming') return;
    this.signal({ type: 'call.reject' });
    this.teardown(null);
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
    const pc = this.pc;
    if (!stream || !pc) return;
    let track = stream.getVideoTracks()[0];
    if (!track) {
      // Audio call: acquire the camera now and renegotiate.
      try {
        const cam = await mediaDevices.getUserMedia({ video: { facingMode: this.facing } });
        track = cam.getVideoTracks()[0];
        stream.addTrack(track);
        pc.addTrack(track, stream);
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
      track._switchCamera();
    }
    this.bump();
  }

  clearError() {
    this.set({ error: null });
  }

  // ---- internals
  private async getLocalMedia(video: boolean): Promise<MediaStream> {
    return mediaDevices.getUserMedia({ audio: true, video: video ? { facingMode: this.facing } : false });
  }

  private createPeer(peerId: number): RTCPeerConnection {
    const pc = new RTCPeerConnection({ iceServers: ICE_SERVERS });
    this.pc = pc;
    this.makingOffer = false;
    this.ignoreOffer = false;

    pc.onnegotiationneeded = async () => {
      try {
        this.makingOffer = true;
        await pc.setLocalDescription();
        this.signal({ type: 'call.offer', sdp: sdpOf(pc.localDescription), video: this.state.video });
      } catch (e) {
        console.warn('negotiation failed', e);
      } finally {
        this.makingOffer = false;
      }
    };

    pc.onicecandidate = (ev: any) => {
      if (ev.candidate) this.signal({ type: 'call.ice', candidate: ev.candidate.toJSON() });
    };

    pc.ontrack = (ev: any) => {
      const stream: MediaStream | undefined = ev.streams?.[0];
      if (!stream) return;
      const known = this.state.remoteStreams.some((s) => s.id === stream.id);
      this.set({ remoteStreams: known ? [...this.state.remoteStreams] : [...this.state.remoteStreams, stream] });
      this.bump();
    };

    pc.onconnectionstatechange = () => {
      switch (pc.connectionState) {
        case 'connected':
          if (this.ringTimer) clearTimeout(this.ringTimer);
          this.set({ status: 'active' });
          break;
        case 'failed':
        case 'closed':
          if (this.pc === pc) this.end('Connection lost', false);
          break;
      }
    };
    void peerId;
    return pc;
  }

  private signal(msg: Record<string, unknown>) {
    const to = this.state.peer?.id;
    if (to) socket.send({ ...msg, to });
  }

  private async onOffer(e: WSEvent) {
    const { from, from_user, sdp, video } = e;
    const st = this.state;
    if (st.status === 'idle') {
      this.polite = true;
      this.pendingOffer = sdp;
      this.pendingCandidates = [];
      this.set({ ...IDLE, status: 'incoming', peer: from_user, video: !!video });
      this.onIncoming?.();
      return;
    }
    if (st.peer?.id !== from) {
      socket.send({ type: 'call.busy', to: from });
      return;
    }
    if (st.status === 'incoming') {
      this.pendingOffer = sdp; // caller renegotiated while ringing; keep the newest offer
      return;
    }
    const pc = this.pc;
    if (!pc) return;
    const collision = this.makingOffer || pc.signalingState !== 'stable';
    this.ignoreOffer = !this.polite && collision;
    if (this.ignoreOffer) return;
    try {
      await pc.setRemoteDescription(new RTCSessionDescription(sdp));
      await this.flushCandidates();
      await pc.setLocalDescription();
      this.signal({ type: 'call.answer', sdp: sdpOf(pc.localDescription) });
    } catch (err) {
      console.warn('offer handling failed', err);
    }
  }

  private async onAnswer(e: WSEvent) {
    const pc = this.pc;
    if (!pc || e.from !== this.state.peer?.id || pc.signalingState !== 'have-local-offer') return;
    try {
      await pc.setRemoteDescription(new RTCSessionDescription(e.sdp));
      await this.flushCandidates();
      if (this.state.status === 'outgoing') this.set({ status: 'connecting' });
    } catch (err) {
      console.warn('answer handling failed', err);
    }
  }

  private async onIce(e: WSEvent) {
    if (e.from !== this.state.peer?.id) return;
    const pc = this.pc;
    if (!pc || !pc.remoteDescription) {
      this.pendingCandidates.push(e.candidate);
      return;
    }
    try {
      await pc.addIceCandidate(new RTCIceCandidate(e.candidate));
    } catch (err) {
      if (!this.ignoreOffer) console.warn('addIceCandidate failed', err);
    }
  }

  private async flushCandidates() {
    const pc = this.pc;
    if (!pc) return;
    const list = this.pendingCandidates;
    this.pendingCandidates = [];
    for (const c of list) {
      try {
        await pc.addIceCandidate(new RTCIceCandidate(c));
      } catch (err) {
        console.warn('addIceCandidate failed', err);
      }
    }
  }

  private onRemoteEnd(e: WSEvent, reason: string | null) {
    if (this.state.status === 'idle' || e.from !== this.state.peer?.id) return;
    this.teardown(reason);
  }

  private end(reason: string | null, notifyPeer: boolean) {
    if (notifyPeer && this.state.peer) {
      this.signal({ type: this.state.status === 'incoming' ? 'call.reject' : 'call.hangup' });
    }
    this.teardown(reason);
  }

  private teardown(reason: string | null) {
    if (this.ringTimer) clearTimeout(this.ringTimer);
    this.ringTimer = null;
    const { localStream } = this.state;
    localStream?.getTracks().forEach((t) => t.stop());
    try {
      this.pc?.close();
    } catch {
      /* already closed */
    }
    this.pc = null;
    this.pendingOffer = null;
    this.pendingCandidates = [];
    this.set({ ...IDLE, error: reason, version: this.state.version + 1 });
  }
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
