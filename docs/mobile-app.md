# The app

The app is an Expo Router project in [`mobile/`](../mobile), written in strict
TypeScript, that runs as a native development build on Android and iOS and as a
web page through react-native-web. This page explains how it is organised; the
subject pages explain each feature's screens in context.

## Stack

| Piece | Version | Role |
| --- | --- | --- |
| Expo SDK | 57 | The runtime, the native modules, the build pipeline (`expo run:android`, `expo prebuild`, `expo export`). |
| React Native | 0.86 (new architecture on) | The UI framework; react-native-web renders the same components in the browser. |
| React | 19.2 | Components, `useSyncExternalStore` for the stores, `useEffectEvent` where a listener needs the latest props. |
| Expo Router | 57 | File-based routing over React Navigation: a native stack at the root, bottom tabs (or a sidebar) inside. |
| TypeScript | 6 | `strict`; `@/` maps to `src/`. |

The app needs a **development build**, not Expo Go, because `react-native-webrtc`
is a native module. `android` and `ios` in the dev shell build one
([development.md](development.md)).

## Layout of `src/`

```
src/
  app/                      routes (one file = one screen; _layout.tsx = navigator)
    _layout.tsx             providers, the root stack, the access guards
    (tabs)/_layout.tsx      the five tabs (sidebar on wide windows)
    (tabs)/index.tsx        Feed        (tabs)/friends.tsx   Friends
    (tabs)/groups.tsx       Groups      (tabs)/messages.tsx  Messages
    (tabs)/profile.tsx      Me
    login.tsx register.tsx change-password.tsx edit-profile.tsx
    user/[id].tsx post/[id].tsx group/[id].tsx chat/[id].tsx chat/group/[id].tsx
    new-group-chat.tsx call.tsx
  components/               screen bodies, cards, rows, the composer, the UI kit (ui.tsx)
  lib/                      api, ws, events, auth, stores, hooks, the call manager, notifications, theme, helpers
  theme.ts                  colours, spacing, radii, reaction emoji
```

Route files are deliberately thin. A screen that also appears as a pane on wide
windows has its body in `components/` (`ChatView`, `GroupView`, `PostView`,
`UserView`, `ProfileEditor`, `PasswordForm`) and the route file only reads its
params, decides between the screen and a redirect, and renders the body.

## The root layout

[`app/_layout.tsx`](../mobile/src/app/_layout.tsx) nests the providers:
`SafeAreaProvider` → `ThemeProvider` → `AuthProvider` → `RootNavigator`, which
wraps a `CallProvider` (so an incoming call can open `/call`) and the
`StatusBar` around the root `Stack`.

The stack's `screenLayout` wraps every screen in a `WideColumn` (except the tabs,
which lay themselves out, and the call, which fills the window) and mounts a
`ToastHost` inside each screen, because on Android a pushed native screen paints
over root-level overlays.

Access is decided by `Stack.Protected` groups, evaluated from `useAuth()`:

| Group | Guard | Screens |
| --- | --- | --- |
| signed in | `user && !user.must_change_password` | `(tabs)`, `post/[id]`, `user/[id]`, `edit-profile`, `group/[id]`, `chat/[id]`, `chat/group/[id]`, `new-group-chat`, `call` |
| signed out | `!user` | `login`, `register` |
| any user | `user` | `change-password` (listed last, so a temporary password lands here) |

Detail routes are `dangerouslySingular`: opening a post or profile that is
already in the stack goes to it instead of stacking a copy. The call screen is a
full-screen modal with the back gesture disabled.

The root layout also subscribes to notification taps and pushes the chat they
name ([notifications.md](notifications.md)).

## The tabs

[`(tabs)/_layout.tsx`](../mobile/src/app/(tabs)/_layout.tsx) declares Feed,
Friends, Groups, Messages (with the unread badge from the unread store) and Me.
On a wide window the tab bar moves to the left as a material sidebar with
labels beside the icons, every tab gets a 1200 px column, and header titles are
centred over it.

## Wide windows

[`lib/layout.ts`](../mobile/src/lib/layout.ts) defines the rule: a window at
least 900 px wide (`useWide()`) is a laptop, a desktop or a tablet on its side.
On such a window:

- `WideColumn` centres a screen in a readable column (760 px, or the width a
  layout asks for) instead of stretching it.
- Each tab keeps its list in a `Panes` layout (a 340 px list pane, or 40 % for
  the feed, beside a detail pane) and shows the open item next to it. The
  selection lives in the URL so reloads and links land on it:

  | Tab | Param | Pane content | Stand-alone route that redirects into it |
  | --- | --- | --- | --- |
  | Feed | `?post=<id>` | `PostView` | `/post/<id>` |
  | Friends | `?user=<id>` | `UserView` | `/user/<id>` (your own id goes to Me instead) |
  | Groups | `?group=<id>` | `GroupView` | `/group/<id>` |
  | Messages | `?chat=u:<id>` or `g:<id>` | `ChatView` | `/chat/<id>`, `/chat/group/<id>` |
  | Me | `?show=edit-profile` or `change-password` | `ProfileEditor` or `PasswordForm`, else your own `UserView` | `/edit-profile`, `/change-password` (unless forced) |

- Rows take `selected` (highlighted) and an `onOpen` that sets the param
  instead of pushing a route; detail components take `embedded` to skip
  configuring a navigator header they do not have (the chat draws a
  `PaneHeader` instead) and `onGone`/`onDone` callbacks to close the pane.

On phones the same files render the plain list and push full-screen routes.
Nothing is duplicated: the route file decides with one `if (wide)`.

## State

| Kind | Mechanism | Examples |
| --- | --- | --- |
| The session | `AuthContext` ([`auth.tsx`](../mobile/src/lib/auth.tsx)) | `user: Me`, `login`, `register`, `logout`, `update`. |
| App-wide values outside React | `createStore` ([`store.ts`](../mobile/src/lib/store.ts)): a value, `set`, and a `use()` hook on `useSyncExternalStore` | the unread total, who is online, the call state (the manager has the same shape). |
| Theme | `ThemeContext` ([`theme.tsx`](../mobile/src/lib/theme.tsx)); preference stored under `ami.theme` | `colors`, `scheme`, `preference`. |
| Screen data | hooks that load and stay live | `usePosts`, `useChat`, `useComments`, `useGroup`, `useLiveLoad`, `useUserSearch`. |
| Form and UI state | component `useState` | composers, pickers, tabs within a screen. |

There is no global store of entities: each screen owns what it shows, and the
WebSocket events are applied by whoever is listening. Two rules keep that
honest: a screen applies the server's HTTP reply to its own action at once, and
refetches after a reconnect.

## Networking

### Where the API is

[`lib/config.ts`](../mobile/src/lib/config.ts) computes `API_URL` once:

1. `EXPO_PUBLIC_API_URL`, when set, wins (release builds require it; `build` enforces `https://`).
2. In the browser: in development, the page's host at port `EXPO_PUBLIC_API_PORT`
   (8080), so a page served by Metro on one machine and opened from another still
   finds that machine's server; in production (`-web-dir`), the page's own origin.
3. On a device in development: the machine running Metro (`hostUri`), or the
   emulator loopback (`10.0.2.2` on Android, `localhost` on iOS).
4. On a device in production without the variable: an error at start, on purpose.

`WS_URL` is `API_URL` with `ws(s)://` and `/ws`. `ICE_SERVERS` is the STUN list
for calls.

### The API client

[`lib/api.ts`](../mobile/src/lib/api.ts): `request(method, path, body)` does one
`fetch` with the JSON body and the Bearer token, treats 204 as `undefined`, and
turns any non-2xx into an `ApiError(status, message)` using the server's
`{error}`. A network failure is `ApiError(0, "Cannot reach server at ...")`.
`upload` sends a file as the request body (a platform twin, below). The `api` object lists one function per route, typed
with the shapes in [`lib/types.ts`](../mobile/src/lib/types.ts), which mirror the
server's structs. `mediaUrl(att)` makes an attachment's absolute URL.

### The socket

[`lib/ws.ts`](../mobile/src/lib/ws.ts) and [`lib/events.ts`](../mobile/src/lib/events.ts);
see [websocket.md](websocket.md#the-client).

## Platform twins

Metro resolves `name.web.ts(x)` for the browser and `name.ts(x)` elsewhere. Each
pair exports the same names, so the rest of the code never checks the platform:

| Module | Devices | Browser |
| --- | --- | --- |
| `storage` | `expo-secure-store` (Keychain / Keystore) | `localStorage` |
| `confirm` | `Alert.alert` with a destructive button | `window.confirm` |
| `upload` | `expo-file-system` `File.upload`, streamed by the OS | `fetch(blob:)` → a Blob as the body |
| `save` | download to cache, `expo-media-library` | an `<a download>` click |
| `notifications` | `expo-notifications`, push tokens, app badge | Notification API, `(n)` in the tab title |
| `webrtc` | `react-native-webrtc` classes, `RTCView`, the ReplayKit picker | the browser's WebRTC, `<video>` / `<audio>` |
| `components/ContextMenu` | no menu (hold-to-select and inline controls) | a right-click / ⋯ menu in a portal |

`Platform.OS === 'web'` checks remain in a few places where the difference is a
detail rather than an implementation: hover-only ⋯ buttons, the arrow keys in
the media viewer, the mouse wheel fix for inverted lists, the keyboard behaviour
of the login form.

## The UI kit and the theme

[`components/ui.tsx`](../mobile/src/components/ui.tsx) is the shared vocabulary:
`Button` (primary, secondary, danger, ghost; small; icon; loading), `Input`,
`Avatar` (initials on a name-coloured disc, or the cropped picture, with the
presence dot), `GroupAvatar`, `Row` (a list row with a selected state), `Fab`,
`SectionLabel`, `WideColumn`, `Panes`, `PaneHeader`, `KeyboardScreen`,
`Loading`, `Empty`, `ErrorBanner`, `Card`, `Segmented`.

Colours live in [`theme.ts`](../mobile/src/theme.ts) as two palettes with the
same keys; `useTheme()` gives the active one and `useStyles(makeStyles)`
memoises a `StyleSheet` built from it, which is how every component styles
itself. The preference (light, dark, system) is on the Me tab and persisted;
`expo-system-ui` paints the root background so transitions never flash.

`KeyboardScreen` is the wrapper for any screen with a text box at the bottom
(chat, comments, forms): a bottom safe area plus a `KeyboardAvoidingView`
offset by the real header height, on both platforms, because Android draws edge
to edge and no longer shrinks the window for the keyboard.

Toasts ([`lib/toast.tsx`](../mobile/src/lib/toast.tsx)) are a tiny pub/sub:
`toast.show`, `toast.error`, `toast.success` from anywhere, rendered by the
`ToastHost` of the topmost screen as a banner that fades after 2.2 s.

### Markdown

Post bodies and profile bios are Markdown rendered by `react-native-markdown-display` with a
`markdown-it` parser set to linkify bare URLs and never render HTML
([`lib/markdown.tsx`](../mobile/src/lib/markdown.tsx)); the styles follow the
theme. The composer's toolbar edits the text with pure helpers in
[`lib/markdownEdit.ts`](../mobile/src/lib/markdownEdit.ts): wrap the selection
in markers (bold, italic, code, link) or prefix the line (heading, quote, list).

### The composer

[`components/Composer.tsx`](../mobile/src/components/Composer.tsx) is the one
"write something and submit" box: `rich` adds the Markdown toolbar and picture,
video and link attachments (posts), `media` adds just the picker (chats),
`compact` is the single-line chat and comment form, `initialText` and
`onCancel` make it an editor. It owns its text, selection and attachments
(`useAttachments`), disables the button while uploads are in flight, and clears
itself after a successful `onSubmit`.

### Menus and actions

The same actions are reachable two ways. On devices, holding a message selects
it and shows Edit / Delete links, comments show inline Reply / Edit / Delete,
and a post has small icons. On the web, hovering shows a ⋯ button and
right-click opens the same menu, built from `menuItems(replyItem, editItem,
deleteItem)` in [`components/menu.ts`](../mobile/src/components/menu.ts) and
rendered through a portal by `ContextMenu.web.tsx`.

## Native configuration

[`app.json`](../mobile/app.json) sets the identifiers (`xyz.anthonychavez.ami`),
the permissions (camera, microphone, media projection and notifications on
Android; background audio and VoIP modes on iOS), the Expo plugins with their
permission strings, Hermes minification for Android release builds, and two
local config plugins in [`mobile/plugins/`](../mobile/plugins):

- `withWebRTCScreenShare.js` turns on react-native-webrtc's media-projection
  foreground service in `MainActivity`, which Android 10+ requires for
  `getDisplayMedia`.
- `withSceneLifecycle.js` adopts the UIScene life cycle that iOS 27 requires at
  launch: it declares a scene manifest, adds a `SceneDelegate` and stops the
  `AppDelegate` from starting React Native itself.

The native projects (`mobile/android`, `mobile/ios`) are generated by
`npx expo prebuild`; after changing `app.json`, a plugin or a native dependency,
run `npx expo prebuild --clean` in `mobile/` so the next build picks it up.

## Checks

`npm test` is `tsc --noEmit && eslint . && prettier --check .`; the dev shell's
`test` script runs it after the Go checks. ESLint uses Expo's flat config with
Prettier's rules disabled, Prettier is set to 140 columns, single quotes and
trailing commas. `npm run format` fixes formatting.

## Adding a screen

1. Put the body in `components/` as a function component with explicit props
   (ids, `embedded`, `onGone`), not reading route params itself.
2. Add the route file under `app/`: read params with `useLocalSearchParams`,
   `Redirect` into a tab pane when `useWide()` and a pane makes sense, otherwise
   render the body. Register it in the right `Stack.Protected` group of the root
   layout with a title.
3. Load data through a hook that also subscribes to the events that change it
   (see `usePosts` and `useChat` for the pattern), applies HTTP replies at once,
   and refetches on `open` if staleness would show.
4. Style with `useStyles(makeStyles)` and the kit in `ui.tsx`; keep any
   platform difference in a `.web` twin.
