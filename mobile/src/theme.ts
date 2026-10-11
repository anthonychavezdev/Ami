export const lightColors = {
  bg: '#f4f5f8',
  card: '#ffffff',
  text: '#15171f',
  muted: '#6b7085',
  border: '#e3e5ec',
  accent: '#4f5bd5',
  accentSoft: '#e9ebfb',
  danger: '#d64545',
  success: '#2f9e63',
  dark: '#0f1117',
};

export type ThemeColors = typeof lightColors;

export const darkColors: ThemeColors = {
  bg: '#0f1117',
  card: '#181b24',
  text: '#e9ebf3',
  muted: '#8f94a8',
  border: '#272b38',
  accent: '#7f89ff',
  accentSoft: '#272c4f',
  danger: '#f07171',
  success: '#4fc48c',
  dark: '#0f1117',
};

export const spacing = { xs: 4, sm: 8, md: 12, lg: 16, xl: 24 };
export const radius = { sm: 8, md: 12, lg: 20, pill: 999 };

export const REACTION_EMOJIS = ['👍', '❤️', '😂', '😮', '😢', '😡', '🔥', '🎉', '👏', '🙏', '💯', '🤔', '😍', '🥳', '😎', '🫡'];
export const DEFAULT_REACTION = '👍';
