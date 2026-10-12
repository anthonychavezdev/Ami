import Constants from 'expo-constants';
import * as Device from 'expo-device';
import * as Notifications from 'expo-notifications';
import { AppState, Platform } from 'react-native';

import { api } from './api';

// OS notifications on Android and iOS (notifications.web.ts is the browser twin).
// Messages that arrive over the WebSocket become local notifications; when the
// app is closed the server pushes through Expo, which needs an EAS project id.
Notifications.setNotificationHandler({
  handleNotification: async () => ({ shouldShowBanner: true, shouldShowList: true, shouldPlaySound: true, shouldSetBadge: true }),
});

let pushToken: string | null = null;

export async function setupNotifications() {
  try {
    if (Platform.OS === 'android') {
      await Notifications.setNotificationChannelAsync('messages', { name: 'Messages', importance: Notifications.AndroidImportance.HIGH });
    }
    const { status } = await Notifications.requestPermissionsAsync();
    if (status !== 'granted') return;
    const projectId = Constants.expoConfig?.extra?.eas?.projectId;
    if (!projectId || !Device.isDevice) return; // push needs EAS credentials and a real device
    pushToken = (await Notifications.getExpoPushTokenAsync({ projectId })).data;
    await api.setPushToken(pushToken);
  } catch (e) {
    console.warn('notifications setup failed', e);
  }
}

// Forget this device's push token (logout).
export async function teardownNotifications() {
  if (pushToken) await api.deletePushToken(pushToken).catch(() => {});
  pushToken = null;
  setBadge(0);
}

export async function notify(title: string, body: string, data: Record<string, unknown>) {
  await Notifications.scheduleNotificationAsync({
    content: { title, body, data, sound: true },
    trigger: Platform.OS === 'android' ? { channelId: 'messages' } : null,
  }).catch(() => {});
}

let launchHandled = false;

// Calls handler with the data of a notification the user tapped, including
// the one that launched the app.
export function onNotificationTap(handler: (data: Record<string, any>) => void): () => void {
  const sub = Notifications.addNotificationResponseReceivedListener((r) => handler(r.notification.request.content.data ?? {}));
  if (!launchHandled) {
    launchHandled = true;
    Notifications.getLastNotificationResponseAsync()
      .then((r) => r && handler(r.notification.request.content.data ?? {}))
      .catch(() => {});
  }
  return () => sub.remove();
}

export function setBadge(count: number) {
  Notifications.setBadgeCountAsync(count).catch(() => {});
}

// True while the app is in the background.
export function isHidden() {
  return AppState.currentState !== 'active';
}
