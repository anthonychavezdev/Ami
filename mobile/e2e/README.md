# End-to-end scripts

Python (standard library only) scripts that drive the Android development
build on emulators through `adb` and `uiautomator`, against the dev server's
demo data. They are the checks the Go test suite cannot make: the screens,
WebRTC media and the Android permission dialogs.

Run them from a `devenv shell` with the server, Metro and the emulator(s) up:

```sh
server                          # :8080 with the demo accounts
android                         # once: builds and installs the app on the `ami` emulator
emulator -avd ami2 &            # the call scripts need a second emulator (create it with
                                # `avdmanager create avd -n ami2 -d pixel_7 -k <system image>`)
python3 mobile/e2e/group_chat.py            # luis chats in Fútbol los domingos
python3 mobile/e2e/group_call.py            # luis and ana call, diego joins over the WebSocket, screen share, camera
python3 mobile/e2e/direct_call.py           # luis calls ana from her profile, screen share, hang up
```

`BASE` points at another server, `E2E_OUT` is where screenshots go (default:
the system temp directory). The scripts print one PASS/FAIL line per step
and exit non-zero on any failure.
