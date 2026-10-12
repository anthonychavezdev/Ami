import { useState } from 'react';

// Whether the pointer is over an element (web); spread the props on it.
export function useHovered() {
  const [hovered, setHovered] = useState(false);
  const hoverProps = { onPointerEnter: () => setHovered(true), onPointerLeave: () => setHovered(false) };
  return [hovered, hoverProps] as const;
}
