import { MediaStream, type MediaStreamTrack, RTCIceCandidate, RTCPeerConnection, RTCSessionDescription } from './webrtc';

import type { SDP } from './events';
import type { IceServer } from './types';

// What a connection reports back to the call manager.
export interface PeerEvents {
  onOffer: (sdp: SDP) => void; // we need to (re)negotiate: send this offer
  onAnswer: (sdp: SDP) => void;
  onCandidate: (candidate: unknown) => void;
  onStream: (stream: MediaStream) => void;
  onConnected: () => void;
  onLost: () => void;
}

// One WebRTC connection to another participant, run with the "perfect
// negotiation" pattern: when both sides offer at once the polite one yields.
export class Peer {
  readonly pc: RTCPeerConnection;
  private makingOffer = false;
  private ignoreOffer = false;
  private pending: unknown[]; // ICE candidates that arrived before the remote description

  constructor(
    private readonly polite: boolean,
    private readonly events: PeerEvents,
    earlyCandidates: unknown[],
    iceServers: IceServer[],
  ) {
    this.pending = earlyCandidates;
    this.pc = new RTCPeerConnection({ iceServers });
    this.pc.onnegotiationneeded = () => void this.offer();
    this.pc.onicecandidate = (ev: any) => ev.candidate && events.onCandidate(ev.candidate.toJSON());
    this.pc.ontrack = (ev: any) => ev.streams?.[0] && events.onStream(ev.streams[0]);
    this.pc.onconnectionstatechange = () => {
      if (this.pc.connectionState === 'connected') events.onConnected();
      else if (this.pc.connectionState === 'failed' || this.pc.connectionState === 'closed') events.onLost();
    };
  }

  /** Sends every track of a stream (starts negotiation). */
  addStream(stream: MediaStream) {
    stream.getTracks().forEach((t) => this.pc.addTrack(t, stream));
  }

  addTrack(track: MediaStreamTrack, stream: MediaStream) {
    this.pc.addTrack(track, stream);
  }

  /** Stops sending a stream's tracks. */
  removeStream(stream: MediaStream) {
    const ids = new Set(stream.getTracks().map((t) => t.id));
    this.pc.getSenders().forEach((s) => s.track && ids.has(s.track.id) && this.pc.removeTrack(s));
  }

  private async offer() {
    try {
      this.makingOffer = true;
      await this.pc.setLocalDescription();
      this.events.onOffer(sdpOf(this.pc.localDescription));
    } catch (e) {
      console.warn('negotiation failed', e);
    } finally {
      this.makingOffer = false;
    }
  }

  /** Takes their offer and answers it; `beforeAnswer` may add our tracks first. */
  async acceptOffer(sdp: SDP, beforeAnswer?: () => void) {
    const collision = this.makingOffer || this.pc.signalingState !== 'stable';
    this.ignoreOffer = !this.polite && collision;
    if (this.ignoreOffer) return;
    try {
      await this.pc.setRemoteDescription(new RTCSessionDescription(sdp));
      beforeAnswer?.();
      await this.flushCandidates();
      await this.pc.setLocalDescription();
      this.events.onAnswer(sdpOf(this.pc.localDescription));
    } catch (e) {
      console.warn('offer handling failed', e);
    }
  }

  async acceptAnswer(sdp: SDP) {
    if (this.pc.signalingState !== 'have-local-offer') return;
    try {
      await this.pc.setRemoteDescription(new RTCSessionDescription(sdp));
      await this.flushCandidates();
    } catch (e) {
      console.warn('answer handling failed', e);
    }
  }

  async addCandidate(candidate: unknown) {
    if (!this.pc.remoteDescription) {
      this.pending = [...this.pending, candidate];
      return;
    }
    try {
      await this.pc.addIceCandidate(new RTCIceCandidate(candidate as any));
    } catch (e) {
      if (!this.ignoreOffer) console.warn('addIceCandidate failed', e);
    }
  }

  private async flushCandidates() {
    const list = this.pending;
    this.pending = [];
    for (const c of list) await this.addCandidate(c);
  }

  close() {
    try {
      this.pc.close();
    } catch {
      /* already closed */
    }
  }
}

function sdpOf(d: RTCSessionDescription | null): SDP {
  return { type: d?.type ?? 'offer', sdp: d?.sdp ?? '' };
}
