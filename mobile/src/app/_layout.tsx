import { Stack, useRouter } from 'expo-router';
import { StatusBar } from 'expo-status-bar';
import React, { useCallback, useEffect } from 'react';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { Loading } from '@/components/ui';
import { AuthProvider, useAuth } from '@/lib/auth';
import { CallProvider } from '@/lib/call';
import { onNotificationTap } from '@/lib/notifications';
import { ThemeProvider, useTheme } from '@/lib/theme';
import { ToastHost } from '@/lib/toast';

export default function RootLayout() {
  return (
    <SafeAreaProvider>
      <ThemeProvider>
        <AuthProvider>
          <RootNavigator />
        </AuthProvider>
      </ThemeProvider>
    </SafeAreaProvider>
  );
}

function RootNavigator() {
  const { user, ready } = useAuth();
  const { colors, scheme } = useTheme();
  const router = useRouter();
  const onIncoming = useCallback(() => router.push('/call'), [router]);

  // Tapping a message notification opens that chat.
  useEffect(() => {
    if (!user) return;
    return onNotificationTap((data) => {
      if (data.group_chat) router.push(`/chat/group/${data.group_chat}`);
      else if (data.chat) router.push(`/chat/${data.chat}`);
    });
  }, [user, router]);

  if (!ready) return <Loading />;

  return (
    <CallProvider onIncoming={onIncoming}>
      <StatusBar style={scheme === 'dark' ? 'light' : 'dark'} />
      <Stack
        screenLayout={({ children }) => (
          <>
            {children}
            <ToastHost />
          </>
        )}
        screenOptions={{
          headerTintColor: colors.accent,
          headerTitleStyle: { color: colors.text },
          headerStyle: { backgroundColor: colors.card },
          headerBackButtonDisplayMode: 'minimal', // otherwise iOS shows the route group name, "(tabs)"
          contentStyle: { backgroundColor: colors.bg },
        }}
      >
        <Stack.Protected guard={!!user && !user.must_change_password}>
          <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
          {/* singular: opening a post/profile/chat that is already in the stack goes to it instead of stacking a copy */}
          <Stack.Screen name="post/[id]" options={{ title: 'Post' }} dangerouslySingular />
          <Stack.Screen name="user/[id]" options={{ title: 'Profile' }} dangerouslySingular />
          <Stack.Screen name="edit-profile" options={{ title: 'Edit profile' }} dangerouslySingular />
          <Stack.Screen name="group/[id]" options={{ title: 'Group' }} dangerouslySingular />
          <Stack.Screen name="chat/[id]" options={{ title: 'Chat' }} dangerouslySingular />
          <Stack.Screen name="chat/group/[id]" options={{ title: 'Group chat' }} dangerouslySingular />
          <Stack.Screen name="new-group-chat" options={{ title: 'New group chat' }} dangerouslySingular />
          <Stack.Screen
            name="call"
            options={{ headerShown: false, presentation: 'fullScreenModal', gestureEnabled: false }}
            dangerouslySingular
          />
        </Stack.Protected>
        <Stack.Protected guard={!user}>
          <Stack.Screen name="login" options={{ headerShown: false }} />
          <Stack.Screen name="register" options={{ title: 'Create account' }} />
        </Stack.Protected>
        {/* Last on purpose: with a temporary password every screen above is gated, so this is where the app opens. */}
        <Stack.Protected guard={!!user}>
          <Stack.Screen name="change-password" options={{ title: 'Change password' }} />
        </Stack.Protected>
      </Stack>
    </CallProvider>
  );
}
