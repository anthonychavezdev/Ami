{ pkgs, lib, config, ... }:

let
  root = config.devenv.root;

  # Android API level the app compiles against (see
  # mobile/node_modules/react-native/gradle/libs.versions.toml), and the
  # emulator system-image ABI, which must match the host CPU.
  androidApi = "36";
  buildTools = "36.0.0";
  emulatorAbi = if pkgs.stdenv.hostPlatform.isAarch64 then "arm64-v8a" else "x86_64";
  systemImage = "system-images;android-${androidApi};google_apis;${emulatorAbi}";
in
{
  # Toolchains. Nothing is installed globally: Go, Node and the Android SDK come
  # from the Nix store, and every cache/dependency directory below lives inside
  # this repo.
  languages.go.enable = true;
  languages.javascript = {
    enable = true;
    npm.enable = true;
    directory = "${root}/mobile"; # puts mobile/node_modules/.bin on PATH
  };

  # Android SDK + NDK + emulator + JDK 17, pinned to what Expo 57 / React Native
  # 0.86 build with. AVDs live in ./.android (set up by devenv's android module).
  android = {
    enable = true;
    reactNative.enable = true; # JDK 17, as React Native expects
    platforms.version = [ androidApi ];
    # 36.0.0 is what the app module uses; library modules that don't pin a
    # version get AGP's default (35.0.0), which it can't download into the
    # read-only Nix SDK.
    buildTools.version = [ buildTools "35.0.0" ];
    ndk.version = [ "27.1.12297006" ];
    cmake.version = [ "3.22.1" ];
    systemImageTypes = [ "google_apis" ];
    abis = [ emulatorAbi ];
    googleAPIs.enable = false;
    googleTVAddOns.enable = false;
    extras = [ ];
  };

  packages = [
    pkgs.sqlite   # `sqlite3 server/ami.db` to poke at the database
    pkgs.watchman # faster Metro file watching
    pkgs.cocoapods # iOS native builds (Xcode itself must come from the App Store)
  ];

  env = {
    # Go: module cache, build cache and `go install` binaries under ./.go
    # (devenv's Go module defaults GOPATH to .devenv/state/go; we keep it all in one place)
    GOPATH = lib.mkForce "${root}/.go";
    GOMODCACHE = "${root}/.go/pkg/mod";
    GOCACHE = "${root}/.go/cache";
    GOBIN = "${root}/.go/bin";
    GOFLAGS = "-modcacherw";

    # npm / npx: package cache and "global" installs under ./.npm
    npm_config_cache = "${root}/.npm/cache";
    npm_config_prefix = "${root}/.npm/global";
    npm_config_update_notifier = "false";
    COREPACK_HOME = "${root}/.npm/corepack";

    # Expo / React Native native builds
    EXPO_NO_TELEMETRY = "0";
    GRADLE_USER_HOME = "${root}/.gradle";
    CP_HOME_DIR = "${root}/.cocoapods";
  };

  enterShell = ''
    export PATH="$GOBIN:$npm_config_prefix/bin:$PATH"
    mkdir -p "$GOMODCACHE" "$GOCACHE" "$GOBIN" "$npm_config_cache" "$npm_config_prefix/bin"
  '';

  scripts.setup.exec = ''
    (cd "$DEVENV_ROOT/server" && go mod tidy)
    (cd "$DEVENV_ROOT/mobile" && npm install)
  '';
  scripts.server.exec = ''
    cd "$DEVENV_ROOT/server" && go run . -dev "$@"
  '';
  scripts.mobile.exec = ''
    cd "$DEVENV_ROOT/mobile" && { [ -d node_modules ] || npm install; } && npx expo start "$@"
  '';
  # Build the Android development build, install it on the emulator (created on
  # first run; a USB-connected phone takes precedence) and start Metro.
  scripts.android.exec = ''
    cd "$DEVENV_ROOT/mobile" && { [ -d node_modules ] || npm install; }
    if ! avdmanager list avd -c 2>/dev/null | grep -qx ami; then
      echo "Creating emulator 'ami' (${systemImage})..."
      echo no | avdmanager create avd -n ami -d pixel_7 -k "${systemImage}"
    fi
    npx expo run:android "$@"
  '';
  # Same for the iOS Simulator. Nix cannot provide Xcode, so check for it first.
  scripts.ios.exec = ''
    if ! env -u DEVELOPER_DIR /usr/bin/xcode-select -p 2>/dev/null | grep -q "/Xcode.app/"; then
      echo "Xcode is required for the iOS Simulator. Install it from the App Store, then run:" >&2
      echo "  sudo xcode-select -s /Applications/Xcode.app && sudo xcodebuild -runFirstLaunch" >&2
      exit 1
    fi
    cd "$DEVENV_ROOT/mobile" && { [ -d node_modules ] || npm install; }
    # Xcode must not see the Nix toolchain (DEVELOPER_DIR, SDKROOT, CC/LD and the
    # NIX_* flags garble the link step), so run with a minimal environment:
    # Apple's tools first, then node, npm and pod from this shell. CocoaPods
    # needs a UTF-8 locale.
    exec env -i HOME="$HOME" USER="$USER" TMPDIR="''${TMPDIR:-/tmp}" LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8 \
      PATH="/usr/bin:/bin:/usr/sbin:/sbin:$(dirname "$(command -v node)"):$(dirname "$(command -v npm)"):$(dirname "$(command -v pod)"):$(dirname "$(command -v watchman)")" \
      npm_config_cache="$npm_config_cache" npm_config_prefix="$npm_config_prefix" npm_config_update_notifier=false \
      CP_HOME_DIR="$CP_HOME_DIR" EXPO_NO_TELEMETRY="$EXPO_NO_TELEMETRY" EXPO_PUBLIC_API_URL="''${EXPO_PUBLIC_API_URL:-}" CI="''${CI:-}" \
      npx expo run:ios "$@"
  '';
  # Production artifacts into ./dist: a stripped static server binary and a
  # minified release APK signed with your own keystore (never the debug key).
  # `build server` builds only the backend.
  scripts.build.exec = ''
    set -euo pipefail
    cd "$DEVENV_ROOT" && mkdir -p dist
    echo "» server -> dist/ami-server (set GOOS/GOARCH to cross-compile, e.g. GOOS=linux GOARCH=amd64)"
    (cd server && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../dist/ami-server .)
    if [ "''${1:-}" = server ]; then exit 0; fi
    : "''${EXPO_PUBLIC_API_URL:?set EXPO_PUBLIC_API_URL=https://your.server (it is baked into the app)}"
    case "$EXPO_PUBLIC_API_URL" in https://*) ;; *) echo "EXPO_PUBLIC_API_URL must be https:// in production" >&2; exit 1 ;; esac
    : "''${AMI_KEYSTORE:?path to your release keystore; create one with:
  keytool -genkeypair -v -keystore ami-release.keystore -alias ami -keyalg RSA -keysize 2048 -validity 10000}"
    : "''${AMI_KEYSTORE_PASSWORD:?password of the keystore and key}"
    echo "» android release -> dist/ami-release.apk (API: $EXPO_PUBLIC_API_URL)"
    cd mobile && npx expo prebuild --platform android --no-install
    # phones only: x86/x86_64 are emulator ABIs and would double the APK size
    (cd android && ./gradlew --quiet assembleRelease -PreactNativeArchitectures=arm64-v8a,armeabi-v7a)
    apksigner="$ANDROID_HOME/build-tools/${buildTools}/apksigner"
    "$apksigner" sign --ks "$AMI_KEYSTORE" --ks-pass env:AMI_KEYSTORE_PASSWORD --ks-key-alias "''${AMI_KEY_ALIAS:-ami}" --v4-signing-enabled false \
      --out "$DEVENV_ROOT/dist/ami-release.apk" android/app/build/outputs/apk/release/app-release.apk
    "$apksigner" verify "$DEVENV_ROOT/dist/ami-release.apk"
    ls -la "$DEVENV_ROOT/dist"
  '';

  # `devenv up` runs the Go API with process-compose.
  processes.server.exec = "cd $DEVENV_ROOT/server && go run . -dev -addr :8080 -db ami.db";
}
