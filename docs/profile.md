# Profile and presence

Editing your own profile, profile pictures and the square shown in the circle,
and appearing offline. Code: [`server/profile.go`](../server/profile.go),
[`server/avatar.go`](../server/avatar.go), the app's
[`ProfileEditor`](../mobile/src/components/ProfileEditor.tsx),
[`AvatarEditor`](../mobile/src/components/AvatarEditor.tsx),
[`profileForm.ts`](../mobile/src/lib/profileForm.ts) and the Me tab
([`(tabs)/profile.tsx`](../mobile/src/app/(tabs)/profile.tsx)).

## PUT /api/me

Body: any subset of `{username, display_name, bio, pronouns, avatar_url, avatar_crop}`.
Only the fields present are changed (the input struct uses pointers, so a field
that is absent is nil and left alone, while an empty string is a real value).
Sending nothing is `400 nothing to update`. The response is the `Me` shape.

`profilePatch` validates each field and appends a `column = ?` to a
`columnPatch`, which becomes one `UPDATE users SET ... WHERE id = ?`.

| Field | Rule | Error |
| --- | --- | --- |
| `username` | 3 to 24 of `A-Za-z0-9_`; not one of the reserved names (`deleted`, `admin`, `ami`, `me`, `system`); not anyone else's, in any letter case. | 400, 400, 409 |
| `display_name` | 1 to 50 characters (runes) after trimming. | 400 |
| `bio` | up to 500 characters after trimming. | 400 |
| `pronouns` | up to 30 characters after trimming. | 400 |
| `avatar_url` | empty (no picture) or `/media/<name>` of an uploaded picture (`.jpg`, `.png`, `.gif`, `.webp`) that exists in the media directory. | 400 |
| `avatar_crop` | `{x, y, w, aspect}`; see below. | 400 |

### Changing the username

`newUsername` checks the pattern, the reserved list and, with
`SELECT id FROM users WHERE username = ? COLLATE NOCASE AND id != ?`, that nobody
else has the name. The unique index catches a lost race with a 409 as well.
Changing the username frees the old one at once; the old name no longer logs
in, the new one does ([testing.md](testing.md) covers this).

### The profile picture

A picture is uploaded first through `POST /api/media` ([media.md](media.md)),
then referenced by its `/media/<name>` URL. Animated GIFs stay animated because
the server never re-encodes pictures. `avatar_url: ""` removes the picture and
its crop.

The **crop** is the square of the picture shown in the round avatar:

```go
type AvatarCrop struct {
    X, Y   float64 // top-left corner, as fractions of the picture's width and height
    W      float64 // the side, as a fraction of the width (the side as a fraction of the height is W*Aspect)
    Aspect float64 // the picture's width / height
}
```

`avatarPatch` decides what to store:

- A new picture without a crop gets `defaultCrop(aspect)`: the largest centred
  square. The aspect ratio comes from the file itself (`pictureAspect`, which
  decodes only the header with `image.DecodeConfig`). WebP has no decoder in the
  Go standard library, so its aspect reads as 0 and the default crop assumes a
  square picture.
- A crop sent with or after a picture is validated: `w` in (0, 1], the square
  inside the picture, `aspect` within reason; the picture's real aspect ratio
  overrides the one sent when it is known. Values are rounded to four decimals
  before storing.
- A crop without any picture is `400 no profile picture to position`.

The crop travels with every `User` object as `avatar_crop`, and the app's
`Avatar` component (`ui.tsx`, `cropStyle`) positions the picture so that square
fills the circle; `w === 0` means "whole picture, centred", which is what an
account without a crop gets.

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

**ProfileEditor** keeps a `ProfileForm` (`profileForm.ts`) seeded from `Me`.
`profilePatch(form, user)` computes only the fields that differ, which is what
`PUT /api/me` receives, and `canSave` requires a valid username, a non-empty
display name and at least one change. Picking a picture uses
`expo-image-picker` (pictures only, no system cropping so GIFs stay animated),
uploads it at once through `api.uploadMedia`, refuses anything that is not a
picture, and sets `defaultCrop(width/height)` from the upload's reported size.
Older pictures without a size load once through an invisible `Image` to learn
their aspect ratio. Saving calls `api.updateMe`, updates the auth context with
the returned `Me`, shows a toast and calls `onDone`.

**AvatarEditor** draws the picture dimmed with a draggable circle over it
showing the chosen square, plus zoom in/out buttons. It works in fractions of the
picture, exactly the `AvatarCrop` shape, and `clamp` keeps the square inside the
picture and at least a quarter of the largest possible square.
