import { Redirect, useRouter } from 'expo-router';
import React from 'react';

import { ProfileEditor } from '@/components/ProfileEditor';
import { useWide } from '@/lib/layout';

// The profile editor's page; on a wide window it opens beside the Me tab instead.
export default function EditProfileScreen() {
  const router = useRouter();
  const wide = useWide();
  if (wide) return <Redirect href={{ pathname: '/profile', params: { show: 'edit-profile' } }} />;
  return <ProfileEditor onDone={() => router.back()} />;
}
