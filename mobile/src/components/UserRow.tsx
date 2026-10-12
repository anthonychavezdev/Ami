import { useRouter } from 'expo-router';
import React from 'react';

import { Avatar, Row } from './ui';
import type { User } from '@/lib/types';

// A person in a list; tapping opens their profile unless told otherwise.
export function UserRow({
  user,
  subtitle,
  right,
  onPress,
}: {
  user: User;
  subtitle?: string;
  right?: React.ReactNode;
  onPress?: () => void;
}) {
  const router = useRouter();
  return (
    <Row
      left={<Avatar user={user} size={44} />}
      title={user.display_name}
      subtitle={subtitle ?? `@${user.username}`}
      right={right}
      onPress={onPress ?? (() => router.push(`/user/${user.id}`))}
    />
  );
}
