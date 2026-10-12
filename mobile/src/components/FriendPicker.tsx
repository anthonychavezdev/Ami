import { Feather } from '@expo/vector-icons';
import React from 'react';
import { FlatList, StyleSheet, View } from 'react-native';

import { UserRow } from './UserRow';
import { Empty } from './ui';
import type { User } from '@/lib/types';
import { useStyles, useTheme } from '@/lib/theme';
import type { ThemeColors } from '@/theme';

// A list of friends with a check mark on each one selected.
export function FriendPicker({ friends, selected, onToggle, header, empty }: { friends: User[]; selected: ReadonlySet<number>; onToggle: (id: number) => void; header: React.ReactElement; empty: string }) {
  const styles = useStyles(makeStyles);
  const { colors } = useTheme();
  return (
    <FlatList
      data={friends}
      keyExtractor={(u) => String(u.id)}
      keyboardShouldPersistTaps="handled"
      ListHeaderComponent={header}
      renderItem={({ item }) => {
        const on = selected.has(item.id);
        return (
          <UserRow
            user={item}
            onPress={() => onToggle(item.id)}
            right={
              <View style={[styles.check, on && { backgroundColor: colors.accent, borderColor: colors.accent }]} accessibilityLabel={on ? `${item.display_name} selected` : `${item.display_name} not selected`}>
                {on && <Feather name="check" size={16} color="#fff" />}
              </View>
            }
          />
        );
      }}
      ListEmptyComponent={<Empty text={empty} />}
      contentContainerStyle={{ flexGrow: 1, paddingBottom: 100 }}
    />
  );
}

// The set with one id added or removed.
export const toggled = (set: ReadonlySet<number>, id: number): ReadonlySet<number> => {
  const next = new Set(set);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  return next;
};

const makeStyles = (colors: ThemeColors) => StyleSheet.create({
  check: { width: 26, height: 26, borderRadius: 13, borderWidth: 2, borderColor: colors.border, alignItems: 'center', justifyContent: 'center' },
});
