import { Redirect, Stack, useRouter } from 'expo-router';
import React from 'react';

import { PasswordForm } from '@/components/PasswordForm';
import { useAuth } from '@/lib/auth';
import { useWide } from '@/lib/layout';

// Sets a new password: by choice from the Me tab (on a wide window beside it),
// or because the account was given a temporary password. Then nothing else
// works until this is done: the server refuses every other request and the
// root layout shows only this screen.
export default function ChangePasswordScreen() {
  const router = useRouter();
  const wide = useWide();
  const forced = !!useAuth().user?.must_change_password;
  if (wide && !forced) return <Redirect href={{ pathname: '/profile', params: { show: 'change-password' } }} />;
  return (
    <>
      <Stack.Screen
        options={{ title: forced ? 'Set a new password' : 'Change password', headerBackVisible: !forced, gestureEnabled: !forced }}
      />
      <PasswordForm onDone={() => (forced ? router.replace('/') : router.back())} />
    </>
  );
}
