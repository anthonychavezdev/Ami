# Dependencies

Every third-party piece Ami relies on, what it does here, and why it was
chosen over doing without. The rule of the project is to keep this list short
and to prefer what Expo and the standard library already provide.

## Go

The server is standard library except for three modules ([`server/go.mod`](../server/go.mod)):

| Module | Version | Used for | Why |
| --- | --- | --- | --- |
| `modernc.org/sqlite` | 1.60 | The database driver behind `database/sql` ([`db.go`](../server/db.go)). | A pure-Go translation of SQLite: no cgo, so the binary is static and cross-compiles with `CGO_ENABLED=0`, and `go test` needs no C toolchain. SQLite itself because one file, no service to run, and the write volume of a friends-scale network needs nothing more. |
| `github.com/google/uuid` | 1.6 | Version 7 UUIDs for every row id (`newID` in [`util.go`](../server/util.go)). | Already in the module graph through the SQLite driver. Its v7 generator is monotonic within a process, which keeps ids ordered by creation time, the property every list and read marker relies on. |
| `github.com/gorilla/websocket` | 1.5 | The WebSocket server: the upgrade, framing, ping/pong, read limits ([`ws.go`](../server/ws.go), [`hub.go`](../server/hub.go)). | The standard library has no WebSocket implementation (`golang.org/x/net/websocket` is minimal and unmaintained). gorilla is small, stable and the de facto choice. Its one rule, a single writer per connection, is what `writeLoop` exists for. |

The indirect modules (`modernc.org/libc`, `modernc.org/mathutil`,
`modernc.org/memory`, `github.com/ncruces/go-strftime`,
`github.com/remyoudompheng/bigfft`,
`github.com/dustin/go-humanize`, `github.com/mattn/go-isatty`,
`golang.org/x/sys`) all come with `modernc.org/sqlite`; nothing imports them
directly.

Standard-library packages doing real work, for orientation: `net/http` (the
server, the method-and-pattern router, `MaxBytesReader`, `ServeFile`),
`database/sql`, `crypto/pbkdf2` and `crypto/subtle` (passwords),
`crypto/rand` and `crypto/sha256` (tokens), `encoding/json`, `image` with
`image/png`, `image/jpeg`, `image/gif` (picture sizes, demo pictures; no WebP
decoder exists there), `mime/multipart` (uploads), `reflect` (presence),
`os/signal` and `context` (shutdown), `log`, `flag`.

## The app

Runtime dependencies from [`mobile/package.json`](../mobile/package.json):

| Package | Version | Used for | Why |
| --- | --- | --- | --- |
| `expo` | 57 | The SDK: the runtime, the CLI (`expo start`, `run:android`, `prebuild`, `export`), config plugins. | The way to build a React Native app that targets Android, iOS and the web from one code base without hand-written native projects. |
| `react`, `react-dom` | 19.2 | Components; `react-dom` for the web renderer and the portal of the web context menu. | The framework. `useSyncExternalStore` and `useEffectEvent` are used directly. |
| `react-native` | 0.86 | The UI primitives and platform APIs (`FlatList`, `Pressable`, `AppState`, `Platform`...). | The framework. |
| `react-native-web` | 0.21 | Renders React Native components in the browser. | One code base for the web page; the `.web.tsx` twins cover what it cannot map. |
| `expo-router` | 57 | File-based routing: the root stack, the tabs, `Stack.Protected` guards, `Redirect`, `useLocalSearchParams`, `useFocusEffect`, deep links. | Routes as files keeps navigation declarative and URL-addressable, which the wide two-pane layout relies on. It wraps React Navigation, whose `useHeaderHeight` and `HeaderHeightContext` the keyboard wrapper and the toast use. |
| `react-native-screens`, `react-native-safe-area-context` | 4.26, 5.7 | Native screen containers for the navigator; safe-area insets (`SafeAreaView`, `useSafeAreaInsets`). | Required by Expo Router; the safe-area hooks are used throughout for notches and home indicators. |
| `expo-linking` | 57 | Deep linking and the `ami://` scheme. | Required by Expo Router; not imported directly. |
| `expo-constants` | 57 | `expoConfig.hostUri` (where Metro runs, to find the dev server) and `extra.eas.projectId` (push). | The supported way to read app config at runtime. |
| `expo-device` | 57 | `isDevice`, to skip push registration on emulators. | Push tokens only exist on real devices. |
| `expo-secure-store` | 57 | The session token in the Keychain / Keystore ([`storage.ts`](../mobile/src/lib/storage.ts)). | Tokens belong in the OS secret store, not in plain preferences. |
| `expo-status-bar` | 57 | The status bar style following the theme, and light over the media viewer. | Expo's cross-platform status bar component, which follows the theme without platform code. |
| `expo-system-ui` | 57 | The root view's background colour behind every screen. | Stops the wrong colour flashing during transitions when the theme is dark. |
| `@expo/vector-icons` | 15 | Feather (and a MaterialCommunityIcons quote icon) everywhere. | Bundled icon fonts with Expo, no asset pipeline. |
| `expo-image` | 57 | Avatars (cropped positioning, animated GIFs) and the picture editor. | Faster and more capable than the core `Image`; GIFs animate, `contentFit` is explicit. The core `Image` is still used where a plain picture is enough. |
| `expo-image-picker` | 57 | Picking pictures and videos from the library, for posts, messages and profile pictures. | The system picker with permission handling and multiple selection. |
| `expo-video` | 57 | Playing attached videos inline (first frame) and fullscreen with native controls. | The current Expo video module (`expo-av` is deprecated). |
| `expo-media-library` | 57 | Saving a picture or video to the photo library. | The permissioned way to write to the library. |
| `expo-file-system` | 57 | `File` objects: the Blob for uploads, downloads to the cache before saving. | Expo's `fetch` only accepts its own `File` as a multipart part; the new `File`/`Paths` API is the maintained one. |
| `expo-notifications` | 57 | Local notifications, the Android channel, the badge, push tokens, tap handling. | The Expo way to both local and push notifications, with Expo's push service on the server side. |
| `react-native-webrtc` | 124 | `RTCPeerConnection`, `mediaDevices`, `RTCView`, screen capture on devices. | The only maintained WebRTC implementation for React Native. Its API mirrors the browser's, which is what lets `webrtc.web.tsx` be a thin twin. |
| `@config-plugins/react-native-webrtc` | 15 | Config plugin adding the camera and microphone permissions and settings react-native-webrtc needs to the native projects. | Keeps those native changes declarative in `app.json` instead of hand edits. |
| `expo-build-properties` | 57 | Config plugin turning on minification and resource shrinking for Android release builds. | Smaller release APKs with one line of config. |
| `react-native-markdown-display` | 7 | Rendering post bodies written in Markdown, with theme styles. | The established Markdown renderer for React Native, built on markdown-it, so bare URLs linkify and HTML can be switched off. |
| `punycode` | 2 | Not imported by the app. | markdown-it's linkifier requires Node's `punycode` module, which React Native does not provide; installing it makes the import resolve. |

Development dependencies:

| Package | Used for |
| --- | --- |
| `typescript` 6, `@types/react`, `@types/react-dom` | Type checking (`tsc --noEmit`), strict mode. |
| `eslint` 9, `eslint-config-expo` | Expo's lint rules (React, hooks, imports, TypeScript) in a flat config. |
| `eslint-config-prettier` | Turns off the lint rules that would fight Prettier. |
| `prettier` 3 | Formatting (140 columns, single quotes, trailing commas); checked by `npm test`. |

Local config plugins in [`mobile/plugins/`](../mobile/plugins) are not
dependencies but behave like them: `withWebRTCScreenShare.js` (Android screen
sharing) and `withSceneLifecycle.js` (iOS 27 scene life cycle). See
[mobile-app.md](mobile-app.md#native-configuration).

## Toolchain

Provided by the dev shell ([`devenv.nix`](../devenv.nix)), nothing installed globally:

| Tool | Version | Role |
| --- | --- | --- |
| devenv (Nix) | rolling nixpkgs via `devenv.yaml` | Reproducible shell with every tool below; `direnv` loads it on `cd`. |
| Go | 1.26 | The server. `crypto/pbkdf2` needs Go 1.24, the method-and-pattern router 1.22, the `min`/`max` builtins 1.21. |
| Node, npm | 24, 11 | Metro, the Expo CLI, the checks. |
| JDK 17 | | What React Native's Gradle build expects. |
| Android SDK | platform 36, build tools 36.0.0 and 35.0.0, NDK 27.1, CMake 3.22, a `google_apis` emulator image matching the host CPU | Building the Android app and running the `ami` emulator. |
| watchman | | Faster file watching for Metro. |
| sqlite3 | | Inspecting the database. |
| python3 | 3.14 | The end-to-end scripts (standard library only). |
| CocoaPods | macOS only | iOS native dependencies; Xcode itself comes from the App Store. |
| process-compose | via `devenv up` | Runs the dev server as a supervised process, optional. |

## Adding or upgrading

- **Go**: `go get module@version` in `server/`, then `go mod tidy` (the `setup`
  script runs it). Prefer the standard library; a new module should earn its
  place the way the two above do.
- **The app**: Expo SDK packages must match the SDK, so install them with
  `npx expo install <package>` in `mobile/` rather than `npm install`, which
  picks the version Expo 57 supports. Upgrading Expo itself is
  `npx expo install expo@<version>` followed by `npx expo install --fix` and a
  `npx expo prebuild --clean`. Any change to native dependencies, `app.json` or
  the plugins needs that prebuild and a new development build.
- After either, run `test`, and `android`/`ios` for native changes.
