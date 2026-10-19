import type { Comment } from './types';

export type Threaded = Comment & { depth: number };

// Replies go under their parent, oldest first, like Reddit.
export function thread(comments: Comment[]): Threaded[] {
  const children = new Map<string | null, Comment[]>();
  comments.forEach((c) => children.set(c.parent_id ?? null, [...(children.get(c.parent_id ?? null) ?? []), c]));
  const walk = (parent: string | null, depth: number): Threaded[] =>
    (children.get(parent) ?? []).flatMap((c) => [{ ...c, depth }, ...walk(c.id, depth + 1)]);
  return walk(null, 0);
}

export const addComment = (list: Comment[], c: Comment) => (list.some((x) => x.id === c.id) ? list : [...list, c]);
export const replaceComment = (list: Comment[], c: Comment) => list.map((x) => (x.id === c.id ? c : x));
