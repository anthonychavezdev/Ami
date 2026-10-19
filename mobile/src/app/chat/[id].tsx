import { Redirect, useLocalSearchParams } from 'expo-router';
import React from 'react';

import { ChatView } from '@/components/ChatView';
import { useWide } from '@/lib/layout';

// Direct chat with one friend; on a wide window it opens beside the Messages list instead.
export default function ChatScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const wide = useWide();
  if (wide) return <Redirect href={{ pathname: '/messages', params: { chat: `u:${id}` } }} />;
  return <ChatView kind="user" id={Number(id)} />;
}
