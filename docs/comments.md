# Comments

Threaded comments under a post. Code: the comments half of
[`server/posts.go`](../server/posts.go); in the app
[`useComments.ts`](../mobile/src/lib/useComments.ts),
[`comments.ts`](../mobile/src/lib/comments.ts) (threading),
[`PostView`](../mobile/src/components/PostView.tsx) and
[`CommentRow`](../mobile/src/components/CommentRow.tsx).

## The comment shape

```json
{ "id": 7, "post_id": 14, "parent_id": 5, "author": { "...User" }, "body": "...",
  "created_at": "...", "edited_at": null, "deleted": false }
```

A deleted comment that still has replies is returned with `deleted: true`,
`body: "[deleted]"`, no `edited_at`, and a stand-in author named `[deleted]`
(`deletedUser`), so the thread under it keeps its shape. The username `deleted`
is reserved for that reason ([profile.md](profile.md#changing-the-username)).

## GET /api/posts/{id}/comments

All comments of the post, oldest first (by id), up to 500, with `?after=<id>`
to continue. There is no threading on the server: the client gets a flat list
with `parent_id`s and nests it.

## POST /api/posts/{id}/comments

Body: `{"body": "...", "parent_id": 5}` (`parent_id` optional).

- The body is trimmed and must be 1 to 2,000 bytes.
- The post must exist (404).
- A `parent_id` must name a live comment **of the same post**
  (`400 cannot reply to that comment` otherwise, including replies to placeholders).
- The row is inserted and two things are pushed to the post's audience:
  `comment.created` with the comment, and `post.updated` with the post (its
  `comment_count` changed). The response is `201` with the comment.

Anyone who can see the post can comment; there is no "friends only" rule on
profile posts.

## PUT /api/comments/{id}

Body: `{"body": "..."}`, same length rule. Only the author may edit
(`ownComment`: 404 if missing or deleted, 403 for someone else). `edited_at` is
set and `comment.updated` goes to the audience.

## DELETE /api/comments/{id}

Only the author. `removeComment` decides between two outcomes:

- The comment has replies: it is kept with `deleted_at` set and an empty body,
  and reads as `[deleted]` from now on. Its reply chain stays readable.
- It has no replies: the row is deleted, and `pruneDeletedAncestors` walks up
  the `parent_id` chain removing placeholders that now have nothing under them,
  so a thread that is deleted from the bottom up disappears entirely.

Either way `comment.deleted {post_id, id}` and a `post.updated` (new count) go to
the audience, and the response is 204.

## In the app

`useComments(postId, onGone)` loads the post and its comments together, keeps
them live, and exposes `add`, `edit` and `remove`:

- `comment.created` and `comment.updated` events for this post are applied in
  place; `comment.deleted` refetches the list because the server decides what
  becomes a placeholder and what is pruned; `post.updated` replaces the post
  (counts, reactions); `post.deleted` shows a toast and calls `onGone` (back on
  a phone, closing the pane on a wide window).
- `add` applies the server's reply and then refetches the post for the count,
  rather than adding one locally, so the `post.updated` event cannot double it.

`thread()` in `comments.ts` nests the flat list: a map from parent id to
children, then a depth-first walk that yields each comment with its `depth`.
`CommentRow` indents by depth (capped at six levels) and draws a left border for
replies.

`PostView` renders the post card, the thread and a single compact `Composer`
whose mode is *new*, *reply to X* or *editing* (an `EditBar` above it shows which,
with a cancel button). Tapping the card's Comment button focuses the box.
Actions per comment: *Reply* on any live comment, *Edit* and *Delete* on your
own, as inline links on devices and in the hover ⋯ / right-click menu on the web.
Deleting asks for confirmation.
