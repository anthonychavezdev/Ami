# Media and attachments

Uploading pictures and videos, how they are checked, stored and served, and
how posts and messages reference them. Code:
[`server/media.go`](../server/media.go); in the app
[`useAttachments.ts`](../mobile/src/lib/useAttachments.ts),
[`upload.ts`](../mobile/src/lib/upload.ts), [`save.ts`](../mobile/src/lib/save.ts),
[`AttachmentChips`](../mobile/src/components/AttachmentChips.tsx),
[`Attachments`](../mobile/src/components/Attachments.tsx) and
[`MediaViewer`](../mobile/src/components/MediaViewer.tsx).

## The attachment shape

```json
{ "type": "image", "url": "/media/5f1c...e9.png", "width": 1200, "height": 900 }
{ "type": "video", "url": "/media/0a7b...41.mp4" }
{ "type": "link",  "url": "https://developer.mozilla.org/..." }
```

Uploads are referenced by their `/media/<name>` path; the app prefixes the API
origin (`mediaUrl` in `api.ts`). Links are absolute URLs and are never fetched
by the server.

## POST /api/media

The picture or video is the request body, with its `Content-Type`. Session
required; 30 uploads per minute per user (`uploadLimiter`, `429` beyond).

1. The body is capped at 20 GB (`maxUploadBytes`): a declared `Content-Length`
   over that is `413 file too large (max 20 GB)` at once, and a streamed body
   that grows past it gets the same once the copy fails (`http.MaxBytesReader`).
   An empty body is a 400. The server's read and write timeouts are sized for
   ordinary requests, so the handler keeps extending them while bytes arrive
   (`pacedReader`, at most once a second) and drops a transfer that stalls for
   two minutes (`transferIdle`). A full disk is `507 no space left for uploads`.
2. The first 512 bytes are sniffed. `sniffMedia` trusts `http.DetectContentType`
   when it recognises a supported type; otherwise, for a declared `video/*` type
   whose bytes start with an ISO media `ftyp` box (iPhone MOV/MP4 brands Go does
   not know), it accepts the declared type or falls back to `video/mp4`. Anything
   else is `415 only JPEG, PNG, GIF and WebP pictures or MP4, MOV and WebM videos are accepted`.
   A text file renamed `.png` is refused; the extension is never trusted.
3. The file is written under a random name: 16 random bytes in hex plus the
   extension for the sniffed type, created with `O_EXCL` so a collision could not
   overwrite anything. The sniffed head and the rest of the body are streamed to
   disk; nothing is held in memory.
4. For pictures, `pictureSize` decodes just the header to report `width` and
   `height` (0 for WebP, which the standard library cannot decode; the app then
   assumes 4:3 for layout). Videos carry no size.
5. `201` with the attachment.

Uploads are not tied to a post: the app uploads as soon as a file is picked and
attaches the result when the post or message is sent. A file that is never
attached stays in the media directory; there is no garbage collection.

## GET /media/{name}

Serves a file from the media directory with no session: the names are random
and unguessable, which is what makes profile pictures work in `<img>` tags and
notifications. The name may not contain path separators or start with a dot
(404 otherwise). Responses are `Cache-Control: public, max-age=31536000, immutable`
(a name never changes content) and carry a Content Security Policy with
`sandbox`, so a picture opened as a page cannot run anything. The default
security headers still apply ([security.md](security.md#headers)). The file is
served by `http.ServeFile` (so `Range` requests work, which is how video players
seek) through a `pacedWriter` that extends the write deadline while bytes flow:
a 20 GB video outlasts the server's 60 s `WriteTimeout` many times over.

## Attachments on posts and messages

`cleanAttachments` runs on every post and message write:

- at most 10 attachments, each URL at most 2,048 bytes;
- `image` and `video` must be `/media/<name>` of a file that exists in the
  media directory (so clients cannot point at uploads that were never made or
  at outside URLs);
- `link` must parse as an `http` or `https` URL with a host; it is stored as
  just `{type, url}`;
- any other `type` is refused.

Errors are `400` with the reason. Profile pictures go through a stricter check
(`validAvatar`): pictures only, by extension.

## In the app

- **Picking and uploading**: `useAttachments` opens the system library with
  `expo-image-picker` (pictures and videos, multiple selection up to the 10
  remaining slots, quality 0.85) and uploads each asset in turn through
  `api.uploadMedia`, counting in-flight uploads so the send button waits. Links
  are added from a small URL field. `AttachmentChips` shows what is queued, with
  thumbnails for pictures and a remove button. On devices the file is handed
  to the OS to stream from disk as the request body (`expo-file-system`'s
  `File.upload`; Expo's `fetch` would read it all into memory first); in the
  browser the picker's `blob:` URL is fetched into a file-backed Blob that the
  browser streams (`upload.ts` / `upload.web.ts`).
- **Showing**: `AttachmentList` lays pictures and videos out in a grid (one per
  row when alone, sized from the measured width because percentage widths and
  aspect ratios do not mix in a wrapping row), videos as a first frame with a
  play badge, links as cards that open in the browser. Tapping a picture or
  video opens `MediaViewer`: a fullscreen pager with side arrows, a thumbnail
  strip when there are several, arrow keys on the web, pinch-to-zoom pictures
  on iOS, `expo-video` playback with native controls, and a *Save* button.
- **Saving**: `saveMedia` downloads the file to the cache and adds it to the
  photo library with `expo-media-library` (asking permission first); in the
  browser it triggers a download (`save.ts` / `save.web.ts`).
- **Profile pictures** use the same upload and the crop editor
  ([profile.md](profile.md#the-two-pictures-and-their-crops)).
