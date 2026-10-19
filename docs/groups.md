# Groups

Groups have members with roles, a feed of posts, a chat and a call. Code:
[`server/groups.go`](../server/groups.go); in the app the Groups tab
([`(tabs)/groups.tsx`](../mobile/src/app/(tabs)/groups.tsx)),
[`GroupView`](../mobile/src/components/GroupView.tsx),
[`GroupHeader`](../mobile/src/components/GroupHeader.tsx),
[`useGroup.ts`](../mobile/src/lib/useGroup.ts) and the
[`new-group-chat`](../mobile/src/app/new-group-chat.tsx) screen.

## The shapes

```json
Group:       { "id": "019a0b20-…", "name": "...", "description": "...", "owner_id": "019a0b10-…", "private": false,
               "created_at": "...", "member_count": 4, "my_role": "member" }
GroupDetail: { "group": Group, "members": [ { "...User", "role": "admin" } ], "call": ["019a0b10-…"] }
```

`my_role` is `admin`, `member` or `""` (not a member), from the viewer's side.
`call` lists the ids of members in the group's call right now
([calls.md](calls.md)). Members are ordered admin first, then by join time.

## Roles and rules

- The **owner** created the group and is its only **admin**; the two are the
  same person today (`owner_id`, and the `admin` role row). The owner cannot
  leave; they delete the group instead.
- A **member** can post, read and write the chat, join the call, and leave.
- The admin can add friends, remove any non-owner member, delete any post or
  chat message in the group, and delete the group.
- A **private** group (invite only) is hidden from discovery, cannot be joined,
  and only the admin adds people. That is what the + button on the Messages tab
  creates: a group chat among friends.
- People can only be **added if they are friends of the admin**
  (`friendsToAdd`), whether at creation or later. Anyone can join a public group.

## GET /api/groups

- `?mine=1`: the groups you belong to, by name.
- otherwise: public groups whose name contains `?q=` (any when empty), up to 50,
  by name. Private groups never appear.

## POST /api/groups

Body: `{"name": "...", "description": "...", "private": false, "member_ids": [3, 5]}`.
Name 1 to 60 bytes, description up to 500 after trimming; `member_ids` (up to
100) must all be your friends (`400 you can only add friends`; duplicates and your
own id are ignored). In one transaction: the group row, your `admin` membership,
and each member with their chat read marker at the current state. `201 GroupDetail`.

## GET /api/groups/{id}

The `GroupDetail` as you see it. Any signed-in user can look at any group by
id, private ones included: privacy is enforced on discovery, joining, the chat
and the call, not on the detail or the posts listing.

## DELETE /api/groups/{id}

Owner only. The group's call is ended first (`endGroupCall`), then the row is
deleted and the cascade removes memberships, posts (with their comments and
reactions), messages and read markers. Every former member's devices get a
`messages.read` event with their new unread total, since the group's unread
messages no longer count. 204.

## POST /api/groups/{id}/join

Public groups only (`403 this group is invite only`). `addMember` inserts the
membership (a no-op when already a member) and sets the chat read marker to the
newest message, so earlier messages do not count as unread. `GroupDetail`.

## POST /api/groups/{id}/leave

Any member but the owner (`400 the owner cannot leave; delete the group instead`).
`dropMember` removes the membership and read marker, takes the person out of the
group's call if they were in it, and sends them their new unread total.

## GET /api/groups/{id}/posts

The group's posts, paged. There is no membership check on this listing; posts
in a private group are reachable to someone who knows the group's id. The feed
only includes groups you belong to.

## POST /api/groups/{id}/members

Admin only. Body `{"user_ids": [...]}` of your friends. Each new member gets the
`group.added {group, by}` event, which the Messages tab uses to reload its list.
`GroupDetail`.

## DELETE /api/groups/{id}/members/{uid}

Admin only; the owner cannot be removed (`400`). Same effects as leaving.

## In the app

- **The Groups tab** has *My groups* (reloaded on focus) and *Discover* (a
  search box, debounced). The + button opens a modal to create a group with a
  name, description and an *Invite only* switch; it then navigates to the group.
  On a wide window the lists sit beside the open group, named by `?group=`.
- **GroupView** is the group screen and the pane: `GroupHeader` (name,
  description, the membership line, and the buttons that fit your role: *Join*,
  *Chat*, *Call*, *Video*, *Add members*, *Leave*, *Delete group*), then a
  *Posts* / *Members* segment. Posts use `usePosts(api.groupPosts)` filtered to
  this group, with a rich composer for members; the admin can delete any post.
  Members are `UserRow`s with a *Remove* button for the admin. `useGroup` loads
  the detail and applies the `GroupDetail` that every membership route returns.
  Deleting the group calls `onGone` (back, or clear the pane).
- **new-group-chat** picks friends with check marks. Without `?group=` it
  creates a private group with a name and the chosen members and opens its chat;
  with `?group=<id>` (from *Add members*) it adds the chosen friends who are not
  members yet.
