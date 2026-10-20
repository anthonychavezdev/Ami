import Constants from 'expo-constants';
import { Platform } from 'react-native';

// Where the Go server lives. Ports and addresses are set in devenv.nix (`env`):
//  - EXPO_PUBLIC_API_URL, when set, is the full URL and wins.
//  - In the browser, the host the page was loaded from, so a page served by
//    Metro on one machine and opened from another still reaches that machine's
//    server; the port is EXPO_PUBLIC_API_PORT. The production build (served by
//    ami-server -web-dir) talks to its own origin.
//  - Native development builds: the machine running Metro (its address is in
//    hostUri), falling back to the emulator/simulator loopback.
//  - Native release builds must set EXPO_PUBLIC_API_URL (`build` enforces it).
const API_PORT = process.env.EXPO_PUBLIC_API_PORT || '8080';

function defaultApiUrl(): string {
  if (Platform.OS === 'web') {
    if (!__DEV__) return window.location.origin;
    return `${window.location.protocol}//${window.location.hostname}:${API_PORT}`;
  }
  if (!__DEV__) throw new Error('Production builds must set EXPO_PUBLIC_API_URL (see README, Production).');
  const host = Constants.expoConfig?.hostUri?.split(':')[0];
  if (host && host !== 'localhost' && host !== '127.0.0.1') return `http://${host}:${API_PORT}`;
  return Platform.OS === 'android' ? `http://10.0.2.2:${API_PORT}` : `http://localhost:${API_PORT}`;
}

export const API_URL = (process.env.EXPO_PUBLIC_API_URL || defaultApiUrl()).replace(/\/$/, '');
export const WS_URL = API_URL.replace(/^http/, 'ws') + '/ws';

// What a call uses until the server's /api/ice has answered, and when it
// cannot: a public STUN server. TURN comes from the server (docs/calls.md).
export const ICE_SERVERS = [{ urls: ['stun:stun.l.google.com:19302'] }];
