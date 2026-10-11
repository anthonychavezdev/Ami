import React, { useMemo } from 'react';
import { Platform, StyleSheet } from 'react-native';
import Markdown, { MarkdownIt } from 'react-native-markdown-display';

import { useTheme } from '@/lib/theme';
import { radius, spacing } from '@/theme';

// Bare URLs become links; HTML is never rendered.
const parser = MarkdownIt({ linkify: true, typographer: false, html: false });
const mono = Platform.select({ ios: 'Menlo', default: 'monospace' });

// Renders a post body written in Markdown with the active theme.
export function PostMarkdown({ children }: { children: string }) {
  const { colors } = useTheme();
  const style = useMemo(
    () =>
      StyleSheet.create({
        body: { color: colors.text, fontSize: 16, lineHeight: 22 },
        paragraph: { marginTop: 0, marginBottom: spacing.sm },
        heading1: { fontSize: 22, fontWeight: '800', marginBottom: spacing.sm },
        heading2: { fontSize: 19, fontWeight: '700', marginBottom: spacing.xs },
        heading3: { fontSize: 17, fontWeight: '700' },
        strong: { fontWeight: '700' },
        em: { fontStyle: 'italic' },
        link: { color: colors.accent },
        blockquote: { backgroundColor: colors.bg, borderLeftColor: colors.accent, borderLeftWidth: 3, paddingHorizontal: spacing.md, marginBottom: spacing.sm },
        code_inline: { backgroundColor: colors.bg, color: colors.text, borderRadius: 4, paddingHorizontal: 4, fontFamily: mono },
        code_block: { backgroundColor: colors.bg, color: colors.text, borderRadius: radius.sm, padding: spacing.md, fontFamily: mono, borderWidth: 0 },
        fence: { backgroundColor: colors.bg, color: colors.text, borderRadius: radius.sm, padding: spacing.md, fontFamily: mono, borderWidth: 0 },
        bullet_list: { marginBottom: spacing.sm },
        ordered_list: { marginBottom: spacing.sm },
        hr: { backgroundColor: colors.border },
        table: { borderColor: colors.border },
        tr: { borderColor: colors.border },
      }),
    [colors],
  );
  return (
    <Markdown markdownit={parser} style={style}>
      {children}
    </Markdown>
  );
}
