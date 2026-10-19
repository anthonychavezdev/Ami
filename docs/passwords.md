# Passwords

Changing your own password, and how an operator recovers an account without
email. Code: [`server/sessions.go`](../server/sessions.go) (`changePassword`,
`resetPassword`, `printTemporaryPassword`), the app's
[`PasswordForm`](../mobile/src/components/PasswordForm.tsx) and the
[`change-password`](../mobile/src/app/change-password.tsx) route.

## PUT /api/me/password

Body: `{"current": "...", "new": "..."}`. Session required; this is one of the
three routes allowed while an account carries a temporary password.

1. `new` must be 8 to 128 bytes.
2. `current` is checked against the stored hash exactly like a login; a mismatch
   is `400 {"error":"current password is wrong"}`.
3. In one transaction: the new password is hashed with a fresh salt and the
   current round count, `must_change_password` is cleared, and every session of
   the account **except the one making the request** is deleted. The device that
   changed the password stays signed in; every other device is signed out.
4. The response is the `Me` shape, now with `must_change_password: false`.

In the app, [`PasswordForm`](../mobile/src/components/PasswordForm.tsx) asks for
the current password, the new one twice, and calls `api.changePassword`. On
success it updates the local `Me` and calls `onDone`: `router.back()` when the
form is its own screen, `router.setParams({ show: undefined })` when it sits in
the pane of the Me tab on a wide window, and `router.replace('/')` after a forced
change ([mobile-app.md](mobile-app.md#wide-windows)).

## Resetting a forgotten password

There is no email, so the operator is the recovery channel. On the machine that
has the database:

```sh
./ami-server -db /var/lib/ami/ami.db -reset-password luis
```

In development, from the dev shell: `server -reset-password luis` (the script
passes its arguments through; `-dev` is not needed).

`printTemporaryPassword` opens the database (running any pending migration),
then `resetPassword`:

1. Looks the username up; an unknown name is an error (`no user named "x"`).
2. Generates a temporary password with `temporaryPassword()`: 12 characters in
   three groups of four, like `k7mp-3vq2-x9ht`, from an alphabet of lowercase
   letters and digits without the look-alikes `i`, `l`, `o`, `0` and `1`, so it
   survives being read out loud or written on paper.
3. In one transaction: stores it as the account's password, sets
   `must_change_password = 1`, and deletes **every** session of the account.
4. Prints the password and exits. The running server is not restarted; SQLite's
   WAL mode and the 5 s busy timeout let the two processes share the file.

The operator tells the person the password in person (or any channel they
trust). The temporary password is a real password: anyone who learns it can sign
in once and set a new one, so it should be used promptly.

## The forced change

Logging in with the temporary password works like any login and returns a
session, but the `Me` in the answer has `must_change_password: true`, and the
session is restricted:

- On the server, `auth` refuses every route except `GET /api/me`,
  `PUT /api/me/password` and `POST /api/logout` with
  `403 {"error":"set a new password first"}` ([login.md](login.md#the-auth-wrapper)).
- In the app, the root layout's `Stack.Protected` groups hide every screen but
  `change-password`, which is the last group and therefore where the app opens
  ([mobile-app.md](mobile-app.md#the-root-layout)). The route file renders the
  form with the title *Set a new password*, no back button and no swipe-back,
  the hint "You signed in with a temporary password", and the placeholder
  *Temporary password* for the current one. Even on a wide window it stays a full
  screen, since the Me tab it would otherwise sit beside is locked.
- The form's `current` is the temporary password. Setting a new one clears the
  flag (the response's `Me` updates the auth context), the layout's guard flips,
  and `router.replace('/')` lands on the feed. The temporary password works only
  once because the change replaced it.

## Password upgrades on login

`pbkdf2Iterations` is 600,000 (OWASP's 2023 figure for PBKDF2-HMAC-SHA256). The
first release used 210,000 and that is the column default, so old accounts
carry the lower number until they log in: `login` compares the stored count with
the constant and re-hashes the password at the current count when it is lower.
Raising the constant is therefore a one-line change that takes effect per
account at its next login. Tests lower the constant to 1,000 in `TestMain` so
the suite stays fast.
