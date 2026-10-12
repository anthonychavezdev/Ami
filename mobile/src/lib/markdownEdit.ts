// Text edits behind the Markdown toolbar. Each takes the current text and
// selection and returns the new ones, without touching anything else.
export type Selection = { start: number; end: number };
export type Edit = { text: string; selection: Selection };

// Wrap the selection (or a placeholder) in markers and keep it selected.
export function wrapSelection(text: string, { start, end }: Selection, before: string, after: string, fallback: string): Edit {
  const inner = text.slice(start, end) || fallback;
  return {
    text: text.slice(0, start) + before + inner + after + text.slice(end),
    selection: { start: start + before.length, end: start + before.length + inner.length },
  };
}

// Prefix the current line (heading, quote, list item) and move the caret along.
export function prefixLine(text: string, { start, end }: Selection, prefix: string): Edit {
  const lineStart = text.lastIndexOf('\n', start - 1) + 1;
  const caret = end + prefix.length;
  return { text: text.slice(0, lineStart) + prefix + text.slice(lineStart), selection: { start: caret, end: caret } };
}

export const isHttpUrl = (s: string) => /^https?:\/\/\S+$/i.test(s);
