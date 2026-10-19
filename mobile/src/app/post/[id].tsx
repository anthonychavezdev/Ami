import { Redirect, useLocalSearchParams, useRouter } from 'expo-router';
import React from 'react';

import { PostView } from '@/components/PostView';
import { useWide } from '@/lib/layout';

// A post's page; on a wide window it opens beside the feed instead.
export default function PostScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const wide = useWide();
  if (wide) return <Redirect href={{ pathname: '/', params: { post: id } }} />;
  return <PostView id={id} onGone={() => router.back()} />;
}
