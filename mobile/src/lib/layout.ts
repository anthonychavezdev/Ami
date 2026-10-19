import { useWindowDimensions } from 'react-native';

// Windows at least this wide (laptops, desktops, tablets on their side) get
// the wide layout: tabs in a sidebar and screens in a column of readable width.
export const WIDE_FROM = 900;
export const COLUMN_WIDTH = 760;

export function useWide() {
  return useWindowDimensions().width >= WIDE_FROM;
}
