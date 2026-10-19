# Users and profiles

Finding people and looking at someone's profile. Code:
[`server/users.go`](../server/users.go); in the app the Friends tab's *Find
people* segment ([`(tabs)/friends.tsx`](../mobile/src/app/(tabs)/friends.tsx)),
[`UserView`](../mobile/src/components/UserView.tsx),
[`ProfileHeader`](../mobile/src/components/ProfileHeader.tsx) and the
[`user/[id]`](../mobile/src/app/user/[id].tsx) route.

## The three shapes of a user

| Type | Fields | Where it appears |
| --- | --- | --- |
| `User` | `id`, `username`, `display_name`, `created_at`, `avatar_url` (omitted when empty), `avatar_crop`, `pronouns` (omitted when empty), `online` | Everywhere a person is referenced: post authors, comment authors, message senders, friends lists, members, events. |
| `Me` | `User` plus `invisible`, `bio`, `must_change_password` | Only for the signed-in account: `/api/me`, `/api/login`, `/api/register`, `PUT /api/me`, `PUT /api/me/password`. |
| `UserProfile` | `User` plus `bio`, `friend_status`, `friend_count`, `post_count` | Search results (status only) and profiles. |

`online` is filled in by the server on output for every `User`
([server.md](server.md#presence-in-every-response)); the app keeps it current
afterwards through presence events ([websocket.md](websocket.md#presence)).

`friend_status` is from the viewer's side: `self`, `none`, `outgoing` (you asked),
`incoming` (they asked), `friends` ([friends.md](friends.md#the-states)).

## GET /api/users?q=

Finds up to 25 accounts, other than yours, whose username or display name
contains `q`, ordered by username. The term is used in a `LIKE` pattern with `%`
and `_` escaped, so searching for `a_b` finds that text literally. An empty `q`
lists the first 25 accounts. Each result is a `UserProfile` with only
`friend_status` filled in (no bio or counts), which is what the Find list needs
for its *Add friend* / *Requested* / *Accept* / *Friends* button.

## GET /api/users/{id}

One account as a `UserProfile`: the public fields, the bio, your friendship
status, how many friends they have (accepted friendships in either direction)
and how many profile posts (group posts are not counted). Your own id gives
`friend_status: "self"`. An unknown id is 404.

## GET /api/users/{id}/posts

The account's profile posts, newest first, paged with `?before=` and `?limit=`.
Profile posts are public to any signed-in user, friend or not; group posts never
appear here. Each post comes with your own reaction and the counts as you would
see them ([posts.md](posts.md#the-post-shape)).

## In the app

- **Find people** is the third segment of the Friends tab. `useUserSearch`
  debounces the query by 250 ms, calls `api.searchUsers`, and keeps each result's
  `friend_status` current from `friend.changed` events and from the button's
  own replies. `FriendStatusButton` shows the one action that fits the status.
- **A profile** is `UserView`: it loads `api.user(id)` and the person's posts
  through `usePosts(api.userPosts)` with an `accept` filter so that live
  `post.created` events for other people's posts are ignored. `ProfileHeader`
  shows the picture, names, bio, counts and the friendship buttons (or none on
  your own profile); *Message*, *Call* and *Video* appear once you are friends.
  The profile reloads itself when a `friend.changed` event names this person,
  so acting from a list beside it keeps it right.
- The route `user/[id]` renders `UserView` full screen on phones and, on a wide
  window, redirects to `/friends?user=<id>` so the profile opens beside the
  Friends list ([mobile-app.md](mobile-app.md#wide-windows)). Links from post
  authors, group members and the Me tab's *View my profile* all go through it.
