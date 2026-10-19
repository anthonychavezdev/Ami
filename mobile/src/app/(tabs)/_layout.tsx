import { Feather } from '@expo/vector-icons';
import { Tabs } from 'expo-router';
import React from 'react';
import type { ColorValue } from 'react-native';

import { WideColumn } from '@/components/ui';
import { useWide } from '@/lib/layout';
import { useTheme } from '@/lib/theme';
import { useUnread } from '@/lib/unread';

type FeatherName = React.ComponentProps<typeof Feather>['name'];

const icon = (name: FeatherName) =>
  function TabIcon({ color, size }: { color: ColorValue; size: number }) {
    return <Feather name={name} size={size} color={color} />;
  };

export default function TabsLayout() {
  const { colors } = useTheme();
  const unread = useUnread();
  const wide = useWide(); // sidebar instead of a bottom bar
  return (
    <Tabs
      screenLayout={({ children }) => <WideColumn>{children}</WideColumn>}
      screenOptions={{
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.muted,
        tabBarPosition: wide ? 'left' : 'bottom',
        tabBarVariant: wide ? 'material' : 'uikit', // the side position needs the material variant
        tabBarLabelPosition: wide ? 'beside-icon' : undefined,
        tabBarStyle: wide
          ? { backgroundColor: colors.card, borderRightWidth: 1, borderRightColor: colors.border, minWidth: 200, paddingTop: 8 }
          : { backgroundColor: colors.card, borderTopColor: colors.border },
        headerStyle: { backgroundColor: colors.card },
        headerTitleAlign: wide ? 'center' : undefined, // over the column, not at the far left
        headerTitleStyle: { color: colors.text, fontWeight: '700' },
        headerTintColor: colors.accent,
        sceneStyle: { backgroundColor: colors.bg },
      }}
    >
      <Tabs.Screen name="index" options={{ title: 'Feed', tabBarIcon: icon('home') }} />
      <Tabs.Screen name="friends" options={{ title: 'Friends', tabBarIcon: icon('users') }} />
      <Tabs.Screen name="groups" options={{ title: 'Groups', tabBarIcon: icon('grid') }} />
      <Tabs.Screen
        name="messages"
        options={{
          title: 'Messages',
          tabBarIcon: icon('message-circle'),
          tabBarBadge: unread > 0 ? (unread > 99 ? '99+' : unread) : undefined,
          tabBarBadgeStyle: { backgroundColor: colors.danger, color: '#fff' },
        }}
      />
      <Tabs.Screen name="profile" options={{ title: 'Me', tabBarIcon: icon('user') }} />
    </Tabs>
  );
}
