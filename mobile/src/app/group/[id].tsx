import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import React from 'react';

import { GroupView } from '@/components/GroupView';
import { useWide } from '@/lib/layout';

// A group's page; on a wide window it opens beside the Groups list instead.
export default function GroupScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const wide = useWide();
  if (wide) return <Redirect href={{ pathname: '/groups', params: { group: id } }} />;
  return <GroupView id={Number(id)} onGone={() => router.back()} />;
}
