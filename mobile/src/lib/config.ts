import Constants from 'expo-constants';
import { Platform } from 'react-native';

// Where the Go server lives. Production builds must set
// EXPO_PUBLIC_API_URL=https://host (the `build` command enforces it). In
// development we assume the server runs on the same machine as Metro (its LAN
// address is in hostUri), falling back to the simulator/emulator loopback.
function defaultApiUrl(): string {
  if (!__DEV__) throw new Error('Production builds must set EXPO_PUBLIC_API_URL (see README, Production).');
  const host = Constants.expoConfig?.hostUri?.split(':')[0];
  if (host && host !== 'localhost' && host !== '127.0.0.1') return `http://${host}:8080`;
  return Platform.OS === 'android' ? 'http://10.0.2.2:8080' : 'http://localhost:8080';
}

export const API_URL = (process.env.EXPO_PUBLIC_API_URL ?? defaultApiUrl()).replace(/\/$/, '');
export const WS_URL = API_URL.replace(/^http/, 'ws') + '/ws';

export const ICE_SERVERS = [{ urls: 'stun:stun.l.google.com:19302' }];
