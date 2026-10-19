import { Redirect, useLocalSearchParams } from 'expo-router';
import React from 'react';

import { UserView } from '@/components/UserView';
import { useWide } from '@/lib/layout';

// Someone's profile; on a wide window it opens beside the Friends list instead.
export default function UserScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const wide = useWide();
  if (wide) return <Redirect href={{ pathname: '/friends', params: { user: id } }} />;
  return <UserView id={id} />;
}
