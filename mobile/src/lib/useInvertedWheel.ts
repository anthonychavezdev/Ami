import { type RefObject, useEffect } from 'react';
import { type FlatList, Platform } from 'react-native';

// react-native-web flips inverted lists with a CSS transform, which also
// flips the mouse wheel; this undoes that once the list is on screen.
export function useInvertedWheel(list: RefObject<FlatList<any> | null>, ready: boolean) {
  useEffect(() => {
    if (Platform.OS !== 'web' || !ready) return;
    const node = list.current?.getScrollableNode?.() as HTMLElement | undefined;
    if (!node) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      node.scrollTop -= e.deltaY;
    };
    node.addEventListener('wheel', onWheel, { passive: false });
    return () => node.removeEventListener('wheel', onWheel);
  }, [list, ready]);
}
