# Posts, the feed and reactions

Code: [`server/posts.go`](../server/posts.go) on the server; in the app
[`usePosts.ts`](../mobile/src/lib/usePosts.ts), the Feed tab
([`(tabs)/index.tsx`](../mobile/src/app/(tabs)/index.tsx)),
[`PostCard`](../mobile/src/components/PostCard.tsx),
[`PostView`](../mobile/src/components/PostView.tsx) and the
[`post/[id]`](../mobile/src/app/post/[id].tsx) route.

## The post shape

```json
{
  "id": "019a0c2e-7b3a-7c21-9f4e-6a0d3b1c2e55", "author": { "...User" }, "group_id": null, "group_name": "Fútbol los domingos",
  "body": "Markdown text", "attachments": [{ "type": "image", "url": "/media/ab12.png", "width": 1200, "height": 900 }],
  "created_at": "2026-10-18T...", "edited_at": null,
  "comment_count": 3, "reactions": { "👍": 2, "❤️": 1 }, "my_reaction": "👍"
}
```

Every post is rendered **for a viewer**: `postSelect` takes the viewer's id as
its first parameter to fill `my_reaction`, counts live comments (placeholders
excluded) and aggregates reactions into `{emoji: count}` with SQLite's
`json_group_object`. `group_name` is present for group posts only, `group_id`
is `null` for profile posts. `attachments` is never null.

## Who sees what

- A **profile post** (`group_id` null) is visible to any signed-in user on the
  author's profile, and appears in the feeds of the author and their friends.
- A **group post** appears in the group's listing and in the feeds of its members.
- `GET /api/posts/{id}` returns any post to any signed-in user; visibility is
  enforced on the listings, not on fetching a single post by id.

`postAudience(authorID, groupID)` is the one place that rule lives for events:
the author plus their friends for a profile post, the author plus the group's
members for a group post. Comment events and post events use it.

## GET /api/feed

Your own posts, your friends' profile posts and every post in the groups you
belong to, newest first, paged with `?before=&limit=`. The query is a single
`SELECT` with the friendship and membership subqueries inline:

```sql
WHERE p.id < ?2 AND (
  (p.group_id IS NULL AND (p.author_id = ?1 OR p.author_id IN (<accepted friends of ?1>)))
  OR p.group_id IN (SELECT group_id FROM group_members WHERE user_id = ?1))
ORDER BY p.id DESC LIMIT ?3
```

## POST /api/posts

Body: `{"body": "...", "group_id": 3, "attachments": [...]}` (`group_id` and
`attachments` optional).

- `cleanPostInput` trims the text, requires text or at least one attachment, caps
  the text at 5,000 bytes, and runs the attachments through `cleanAttachments`
  ([media.md](media.md#attachments-on-posts-and-messages)): at most 10, uploads
  that exist, links that are http(s) URLs.
- A `group_id` requires membership (`403 join the group to post in it`).
- The row is inserted, `broadcastPost("post.created", id)` runs, and the
  response is `201` with the post as the author sees it.

## GET /api/posts/{id}

The post as you see it, or 404.

## PUT /api/posts/{id}

Body: `{"body": "...", "attachments": [...]}`, the same checks as creation. Only
the author may edit (`403 only the author can edit a post`); the group admin
cannot. `edited_at` is set and everyone who sees the post gets `post.updated`.

## DELETE /api/posts/{id}

Allowed for the author and for the admin of the group the post is in
(`mayDeletePost`). The audience is computed *before* the row goes (the group
membership is still needed for it), the delete cascades to comments and
reactions, and `post.deleted {id, group_id}` is sent to the audience. 204.

## Reactions

`PUT /api/posts/{id}/reaction` with `{"emoji": "🔥"}` upserts your single
reaction (the table's primary key is `(post_id, user_id)`), so changing it
replaces the previous one. `DELETE /api/posts/{id}/reaction` removes it. Both
answer with the updated post. `validEmoji` accepts any non-empty valid UTF-8
string of at most 32 bytes and 8 runes, so any emoji sequence works; the app
offers a fixed palette (`REACTION_EMOJIS` in `theme.ts`) with 👍 as the tap
default. Reacting to a missing post is 404.

## Live updates: `broadcastPost`

After a create, edit, reaction or comment change, `broadcastPost(type, id)` loads
the post once to learn its audience, then, for each audience member **who is
online right now**, loads the post again *for that viewer* (so their own
`my_reaction` is right) and sends `post.created` or `post.updated` with it.
Offline users simply fetch fresh data when they come back; posts are not pushed
as notifications.

## In the app

### `usePosts(fetchPage, accept)`

One hook serves the feed, a profile and a group. It owns a cursor-paginated list:

- `load(reset)` fetches a page from `fetchPage(before)` and appends it (or
  replaces the list on reset), remembering the oldest id as the cursor and
  whether the last page was empty. A `busy` ref stops overlapping loads.
- `loading` is true until the first page of the *current* `fetchPage` has
  arrived (`loadedFor !== fetchPage`), so switching profiles shows a spinner
  instead of the previous person's posts. The `setLoadedFor(() => fetchPage)`
  form matters: a function passed straight to a state setter would be run as an
  updater.
- Live: `post.created` is passed through `accept` (the feed accepts everything,
  a profile only that author's profile posts, a group only its own) and
  prepended; `post.updated` replaces the post in place; `post.deleted` removes
  it. `accept` is read through `useEffectEvent`, so the latest filter is used
  without re-subscribing.
- `prepend` is used after creating a post, `update` after an edit or a
  reaction, `remove` after a delete, so your own actions show at once even if
  the socket is down.

### The Feed tab

A `FlatList` of `PostCard`s with a rich `Composer` as its header (Markdown
toolbar, pictures, videos, links) and pull-to-refresh. On a wide window the
feed takes 40 % of the width and the open post with its comments sits beside
it, named by `?post=`; the Comment button of a card opens the post there, and
`/post/<id>` redirects into the pane ([mobile-app.md](mobile-app.md#wide-windows)).

### `PostCard`

Shows the author line (a link to the profile, the group name, the age, an
*edited* mark), the body rendered as Markdown ([mobile-app.md](mobile-app.md#markdown)),
the attachments, the reaction summary, a `ReactionButton` (tap for 👍,
press-and-hold for the palette) and the Comment button with the count. The
author can edit in place (the card swaps to a `Composer` seeded with the text and
attachments) and delete after a confirmation; a group admin can delete. On the
web the actions are in a ⋯ button on hover and in the right-click menu; on
devices they are small icons in the header. `useReact` serialises reaction
requests and, on a 404, tells the user the post is gone and removes it.

### The post screen

`PostView` is the post with its comments ([comments.md](comments.md#in-the-app));
the `post/[id]` route renders it full screen on phones and redirects to the feed
pane on wide windows.
