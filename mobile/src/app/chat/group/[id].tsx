import { useLocalSearchParams } from 'expo-router';
import React from 'react';

import { ChatView } from '@/components/ChatView';

// A group's chat.
export default function GroupChatScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  return <ChatView kind="group" id={Number(id)} />;
}
