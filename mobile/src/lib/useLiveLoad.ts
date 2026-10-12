import { useFocusEffect } from 'expo-router';
import { useCallback } from 'react';

import type { EventType } from './events';
import { socket } from './ws';

// Loads something whenever the screen is focused and again after any of the
// events; `events` should be a constant so the effect is not re-run each render.
export function useLiveLoad(load: () => Promise<void>, events: readonly EventType[]) {
  useFocusEffect(
    useCallback(() => {
      void load();
      const offs = events.map((type) => socket.on(type, () => void load()));
      return () => offs.forEach((off) => off());
    }, [load, events]),
  );
}
