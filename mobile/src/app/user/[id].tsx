import { Redirect, useLocalSearchParams } from 'expo-router';
import React from 'react';

import { UserView } from '@/components/UserView';
import { useAuth } from '@/lib/auth';
import { useWide } from '@/lib/layout';

// Someone's profile; on a wide window it opens beside the Friends list instead,
// or, when the someone is you, on the Me tab, which already shows your profile.
export default function UserScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { user: me } = useAuth();
  const wide = useWide();
  if (wide && id === me?.id) return <Redirect href="/profile" />;
  if (wide) return <Redirect href={{ pathname: '/friends', params: { user: id } }} />;
  return <UserView id={id} />;
}
