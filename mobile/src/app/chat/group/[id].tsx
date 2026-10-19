import { Redirect, useLocalSearchParams } from 'expo-router';
import React from 'react';

import { ChatView } from '@/components/ChatView';
import { useWide } from '@/lib/layout';

// A group's chat; on a wide window it opens beside the Messages list instead.
export default function GroupChatScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const wide = useWide();
  if (wide) return <Redirect href={{ pathname: '/messages', params: { chat: `g:${id}` } }} />;
  return <ChatView kind="group" id={id} />;
}
