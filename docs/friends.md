# Friends

Friend requests, the states a pair of people can be in, and what friendship
unlocks. Code: [`server/friends.go`](../server/friends.go); in the app the Friends
tab ([`(tabs)/friends.tsx`](../mobile/src/app/(tabs)/friends.tsx)) and the buttons in
[`ProfileHeader`](../mobile/src/components/ProfileHeader.tsx).

## The states

A pair of accounts has at most one `friendships` row, with `from_id` being
whoever asked. `friendStatus(me, other)` reads it from one side:

| `friend_status` | Meaning for the viewer |
| --- | --- |
| `self` | It is you. |
| `none` | No row. |
| `outgoing` | You asked; they have not answered. |
| `incoming` | They asked you. |
| `friends` | Accepted. |

Friendship is what unlocks direct messages, calls, being added to groups, and
seeing each other's profile posts in the feed. Profiles and profile posts
themselves are visible to any signed-in user.

## GET /api/friends

Your friends as `User` objects, ordered by display name.

## GET /api/friends/requests

`{"incoming": [User...], "outgoing": [User...]}`: who is waiting for your answer
and whom you are waiting on, newest first.

## POST /api/friends/{id}/request

- Yourself: `400 cannot friend yourself`. Unknown id: 404.
- Already friends: `409 already friends`. Already asked: `409 request already sent`.
- They had already asked you: the request **accepts theirs** instead, and the
  answer is `{"friend_status": "friends"}`.
- Otherwise a pending row is inserted and the answer is `{"friend_status": "outgoing"}`.

## POST /api/friends/{id}/accept

Turns their pending request into an accepted friendship. If there is none
(nobody asked, or you were the one asking) the answer is
`404 no pending request from that user`. `{"friend_status": "friends"}`.

## DELETE /api/friends/{id}

One route for cancelling your request, declining theirs and unfriending: the
row for the pair is deleted whichever direction it had, and the answer is
`{"friend_status": "none"}` even when there was nothing to delete.

## The `friend.changed` event

Every change above tells **both** people, the one who acted included, with
`friend.changed {user, status}` where `user` is the other person and `status`
is the receiver's own view (`outgoing` for the asker and `incoming` for the
asked, `friends` for both on accept, `none` for both on removal). Sending it to
the actor too is what keeps a list and a profile pane showing the same person
in step, and keeps the actor's other devices right.

## In the app

The Friends tab has three segments:

- **Friends** and **Requests** load through `useLiveLoad`, which reloads on
  focus and after every `friend.changed` event. Request rows carry *Accept* /
  *Decline* or *Cancel* buttons that call the API and reload.
- **Find people** searches as you type (debounced) and shows a
  `FriendStatusButton` per result: *Add friend*, *Requested* (tap to cancel),
  *Accept*, or a *Friends* check. The result list updates each person's status
  from `friend.changed` events and from the button's own replies, so it never
  shows a stale button.

A profile's `ProfileHeader` shows the buttons for the current state, and
`UserView` reloads the profile when a `friend.changed` event names that person.
On a wide window the tab keeps its lists beside the open profile; tapping a row
sets `?user=` and `/user/<id>` redirects there ([mobile-app.md](mobile-app.md#wide-windows)).
