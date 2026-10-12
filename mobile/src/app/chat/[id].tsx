import { useLocalSearchParams } from 'expo-router';
import React from 'react';

import { ChatView } from '@/components/ChatView';

// Direct chat with one friend.
export default function ChatScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  return <ChatView kind="user" id={Number(id)} />;
}
