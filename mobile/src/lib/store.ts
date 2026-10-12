import { useSyncExternalStore } from 'react';

// A tiny external store: a value, a way to replace it (never mutate it), and
// a hook that re-renders when it changes. Used for app-wide state that lives
// outside React, such as unread counts and who is online.
export function createStore<T>(initial: T) {
  let value = initial;
  const listeners = new Set<() => void>();
  const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => void listeners.delete(listener);
  };
  const get = () => value;
  const set = (next: T | ((previous: T) => T)) => {
    const resolved = typeof next === 'function' ? (next as (previous: T) => T)(value) : next;
    if (Object.is(resolved, value)) return;
    value = resolved;
    listeners.forEach((listener) => listener());
  };
  return { get, set, subscribe, use: () => useSyncExternalStore(subscribe, get, get) };
}
