import React, { useEffect, useRef } from 'react';
import { View } from 'react-native';

// Browser WebRTC, the API react-native-webrtc mirrors. Browsers only expose
// cameras and microphones on secure origins (https or localhost).
export const { RTCPeerConnection, RTCIceCandidate, RTCSessionDescription, MediaStream, MediaStreamTrack } = globalThis;
export const mediaDevices = navigator.mediaDevices;

export const ScreenCapturePicker = () => null;
export const canFlipCamera = false; // desktop cameras do not flip

export function switchCamera(_track: MediaStreamTrack) {}

export interface StreamViewProps {
  stream: MediaStream;
  style: any;
  fit: 'cover' | 'contain';
  mirror?: boolean;
  zOrder?: number;
  muted?: boolean;
}

// Video only: RemoteAudio plays the sound so a stream is never heard twice.
export function StreamView({ stream, style, fit, mirror }: StreamViewProps) {
  const ref = useRef<HTMLVideoElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.srcObject = stream;
  }, [stream]);
  return (
    <View style={style}>
      <video
        ref={ref}
        autoPlay
        playsInline
        muted
        style={{ width: '100%', height: '100%', objectFit: fit, transform: mirror ? 'scaleX(-1)' : undefined, backgroundColor: '#000' }}
      />
    </View>
  );
}

export function RemoteAudio({ streams }: { streams: MediaStream[] }) {
  return (
    <>
      {streams.map((s) => (
        <Audio key={s.id} stream={s} />
      ))}
    </>
  );
}

function Audio({ stream }: { stream: MediaStream }) {
  const ref = useRef<HTMLAudioElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.srcObject = stream;
  }, [stream]);
  return <audio ref={ref} autoPlay />;
}
