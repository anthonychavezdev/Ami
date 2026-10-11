{ pkgs, lib, config, ... }:

let
  root = config.devenv.root;
in
{
  # Toolchains. Nothing is installed globally: Go and Node come from the Nix
  # store, and every cache/dependency directory below lives inside this repo.
  languages.go.enable = true;
  languages.javascript = {
    enable = true;
    npm.enable = true;
    directory = "${root}/mobile"; # puts mobile/node_modules/.bin on PATH
  };

  packages = [
    pkgs.sqlite   # `sqlite3 server/social.db` to poke at the database
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

  # `devenv up` runs the Go API with process-compose.
  processes.server.exec = "cd $DEVENV_ROOT/server && go run . -addr :8080 -db social.db";
}
