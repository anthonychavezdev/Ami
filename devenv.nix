{ pkgs, lib, config, ... }:

let
  root = config.devenv.root;

  # Android API level the app compiles against (see
  # mobile/node_modules/react-native/gradle/libs.versions.toml), and the
  # emulator system-image ABI, which must match the host CPU.
  androidApi = "36";
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
    buildTools.version = [ "36.0.0" "35.0.0" ];
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
    cd "$DEVENV_ROOT/server" && go run . "$@"
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

  # `devenv up` runs the Go API with process-compose.
  processes.server.exec = "cd $DEVENV_ROOT/server && go run . -addr :8080 -db ami.db";
}
