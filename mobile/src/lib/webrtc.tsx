import React from 'react';
import { MediaStream, MediaStreamTrack, RTCIceCandidate, RTCPeerConnection, RTCSessionDescription, RTCView, ScreenCapturePickerView, mediaDevices } from 'react-native-webrtc';

// WebRTC primitives for Android and iOS; webrtc.web.tsx is the browser twin
// with the same exports, so the call manager runs unchanged on the web.
export { MediaStream, MediaStreamTrack, RTCIceCandidate, RTCPeerConnection, RTCSessionDescription, mediaDevices };

export const ScreenCapturePicker = ScreenCapturePickerView as React.ComponentType<any>;
export const canFlipCamera = true;

export function switchCamera(track: MediaStreamTrack) {
  track._switchCamera();
}

export interface StreamViewProps {
  stream: MediaStream;
  style: any;
  fit: 'cover' | 'contain';
  mirror?: boolean;
  zOrder?: number;
  muted?: boolean;
}

export function StreamView({ stream, style, fit, mirror, zOrder }: StreamViewProps) {
  return <RTCView streamURL={stream.toURL()} style={style} objectFit={fit} mirror={mirror} zOrder={zOrder} />;
}

// Native audio plays through the call audio session; nothing to render.
export function RemoteAudio(_: { streams: MediaStream[] }) {
  return null;
}
