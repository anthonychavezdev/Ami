import { useCallback, useEffect, useState } from 'react';

import { api } from './api';
import { toast } from './toast';
import type { Group, GroupDetail, GroupMember } from './types';

// A group and its members, plus the membership actions that return the
// updated group (join, leave, add, remove).
export function useGroup(groupId: number) {
  const [group, setGroup] = useState<Group | null>(null);
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [error, setError] = useState<string | null>(null);

  const apply = (detail: GroupDetail) => {
    setGroup(detail.group);
    setMembers(detail.members);
  };

  const load = useCallback(async () => {
    try {
      apply(await api.group(groupId));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [groupId]);

  useEffect(() => void load(), [load]);

  // Runs a membership change and shows the result, or the error as a toast.
  const change = async (fn: () => Promise<GroupDetail>) => {
    try {
      apply(await fn());
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    }
  };

  return { group, members, error, reload: load, change };
}
