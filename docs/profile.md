# Profile and presence

Editing your own profile: names, a Markdown bio, interests, links, the profile
picture and the cover with the window of each that is shown, and appearing
offline. Code: [`server/profile.go`](../server/profile.go),
[`server/pictures.go`](../server/pictures.go), the app's
[`ProfileEditor`](../mobile/src/components/ProfileEditor.tsx),
[`CropEditor`](../mobile/src/components/CropEditor.tsx),
[`Cover`](../mobile/src/components/Cover.tsx),
[`ProfileHeader`](../mobile/src/components/ProfileHeader.tsx),
[`profileForm.ts`](../mobile/src/lib/profileForm.ts) and the Me tab
([`(tabs)/profile.tsx`](../mobile/src/app/(tabs)/profile.tsx)).

## PUT /api/me

Body: any subset of `{username, display_name, bio, pronouns, interests, links, avatar_url, avatar_crop, cover_url, cover_crop}`.
Only the fields present are changed (the input struct uses pointers, so a field
that is absent is nil and left alone, while an empty string is a real value).
Sending nothing is `400 nothing to update`. The response is the `Me` shape.

`profilePatch` validates each field and appends a `column = ?` to a
`columnPatch`, which becomes one `UPDATE users SET ... WHERE id = ?`.

| Field | Rule | Error |
| --- | --- | --- |
| `username` | 3 to 24 of `A-Za-z0-9_`; not one of the reserved names (`deleted`, `admin`, `ami`, `me`, `system`); not anyone else's, in any letter case. | 400, 400, 409 |
| `display_name` | 1 to 50 characters (runes) after trimming. | 400 |
| `bio` | up to 500 characters after trimming. Markdown: the app renders it, the server keeps the text. | 400 |
| `pronouns` | up to 30 characters after trimming. | 400 |
| `interests` | an array of tags: trimmed, empty ones and repeats (in any letter case) dropped, each up to 30 characters, up to 20 in all. | 400 |
| `links` | an array of `{label, url}`, up to 10: the URL must be `http` or `https` with a host, like a post's link attachment; the label is trimmed, up to 30 characters, and defaults to the site's host without `www.`. | 400 |
| `avatar_url`, `cover_url` | empty (no picture) or `/media/<name>` of an uploaded picture (`.jpg`, `.png`, `.gif`, `.webp`) that exists in the media directory. | 400 |
| `avatar_crop`, `cover_crop` | `{x, y, w, aspect}`; see below. | 400 |

### Changing the username

`newUsername` checks the pattern, the reserved list and, with
`SELECT id FROM users WHERE username = ? COLLATE NOCASE AND id != ?`, that nobody
else has the name. The unique index catches a lost race with a 409 as well.
Changing the username frees the old one at once; the old name no longer logs
in, the new one does ([testing.md](testing.md) covers this).

### The two pictures and their crops

A profile has a round **profile picture** and a wide **cover** across the top
of the page. Each is a picture uploaded first through `POST /api/media`
([media.md](media.md)) and referenced by its `/media/<name>` URL, plus a
**crop**: the window of the picture that is shown. Animated GIFs stay animated
because the server never re-encodes pictures. `avatar_url: ""` or
`cover_url: ""` removes that picture and its crop.

```go
type Crop struct {
    X, Y   float64 // top-left corner, as fractions of the picture's width and height
    W      float64 // the window's width, as a fraction of the picture's width
    Aspect float64 // the picture's width / height
}
```

The window's shape is fixed per picture (`picture.ratio` in `pictures.go`): 1
for the profile picture, a square shown in a circle, and 3 for the cover, which
is three times as wide as it is tall. So the window's height as a fraction of
the picture's height is `W*Aspect/ratio`. `picturePatch` decides what to store
for either picture:

- A new picture without a crop gets `defaultCrop(aspect, ratio)`: the largest
  centred window of that shape. The aspect ratio comes from the file itself
  (`pictureAspect`, which decodes only the header with `image.DecodeConfig`).
  WebP has no decoder in the Go standard library, so its aspect reads as 0 and
  the default crop assumes the picture has the window's shape.
- A crop sent with or after a picture is validated: `w` in (0, 1], the window
  inside the picture, `aspect` within reason; the picture's real aspect ratio
  overrides the one sent when it is known. Values are rounded to four decimals
  before storing.
- A crop without any picture is `400 no picture to position`.

The profile picture and its crop travel with every `User` object (`avatar_url`,
`avatar_crop`), since people appear everywhere; the cover only comes with the
profile and with `Me`. The app's `cropStyle` (`ui.tsx`) positions a picture so
its window fills a box: the `Avatar` circle or the `Cover` band. `w === 0` means
"whole picture, filling the box", which is what a picture without a crop gets.

### Interests and links

Interests are plain tags. They show as chips on the profile, and
`GET /api/users?q=` matches them as well as names, so tapping a chip on
someone's profile finds everyone into the same thing ([users.md](users.md)).
Links are the person's other places (a website, Instagram, GitHub...), shown
as chips that open in the browser; the label is free text and defaults to the
host. Both are stored as JSON text in the `users` row.

## PUT /api/me/presence

Body: `{"invisible": true}`. Stores the flag and calls `hub.setHidden`: if the
account is connected, everyone receives a `presence` event saying it went
offline (or came back online when the flag is cleared), and from then on the
account reads as offline in every response and is left out of `hello`'s online
list. The account still sees everyone else's presence. Details in
[websocket.md](websocket.md#presence).

## In the app

**The Me tab** ([`(tabs)/profile.tsx`](../mobile/src/app/(tabs)/profile.tsx)) shows
the avatar, names and the actions: *Edit profile*, *View my profile* (phones
only), *Change password*, *Log out*, the theme choice (light, dark, system) and
the *Appear offline* switch, which calls `api.setInvisible` and updates the local
`Me`. On a wide window the settings sit in a pane beside your own profile
(`UserView`), and the two forms open in that pane instead of as screens, named
by `?show=edit-profile` or `?show=change-password` ([mobile-app.md](mobile-app.md#wide-windows)).

**ProfileEditor** keeps a `ProfileForm` (`profileForm.ts`) seeded from `Me`:
the cover, the picture, display name, username, pronouns, the bio (with a hint
that Markdown works), interests as chips with an "add" box that also accepts a
comma-separated list, and links as rows of label and URL. `profilePatch(form,
user)` computes only the fields that differ, which is what `PUT /api/me`
receives, and `canSave` requires a valid username, a non-empty display name,
an http(s) URL on every link and at least one change. Picking either picture
uses `expo-image-picker` (pictures only, no system cropping so GIFs stay
animated), uploads it at once through `api.uploadMedia`, refuses anything that
is not a picture, and sets `defaultCrop(width/height, ratio)` from the upload's
reported size. Older pictures without a size load once through an invisible
`Image` to learn their aspect ratio. Saving calls `api.updateMe`, updates the
auth context with the returned `Me`, shows a toast and calls `onDone`.

**CropEditor** draws a picture dimmed with a draggable window over it showing
the chosen part, a circle for the profile picture or a wide box for the cover,
plus zoom in/out buttons. It works in fractions of the picture, exactly the
`Crop` shape, and `clamp` keeps the window inside the picture and at least a
quarter of the largest possible one.

**ProfileHeader** shows the cover (with the avatar overlapping its bottom
edge), the names, the bio rendered with the same Markdown component as posts,
the interest chips (each a search), the link chips, the counts and the
friendship buttons.
