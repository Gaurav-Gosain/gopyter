{
  description = "gopyter - a Jupyter-style notebook for Go that runs in the terminal";

  # gopyter compiles cells with the user's own Go toolchain and talks to the
  # terminal through pure-Go code, so it cross-compiles. nixpkgs-unstable is
  # pinned because a nixpkgs release from before Go 1.27 cannot build it.
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs =
    { self, nixpkgs }:
    let
      # The release workflow ships linux and darwin on amd64 and arm64 (see
      # .goreleaser.yaml), but x86_64-darwin is missing here on purpose:
      # nixpkgs 26.11 dropped Intel Mac support, so a flake pinned to a
      # current nixpkgs cannot build there. Intel Mac users install with
      # install.sh or `go install`, which still ship darwin/amd64.
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      forAllSystem = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          # A flake cannot read its own git tag, and a flake taken from a
          # dirty checkout or a bare path has no revision at all, so both are
          # reported as "devel" rather than failing to evaluate. The
          # GoReleaser builds report the real version.
          rev = if self ? shortRev then self.shortRev else "devel";
        in
        {
          gopyter =
            # nixpkgs keeps the unversioned `go` on the previous stable
            # release (1.26) and exposes the newest one as `go_latest`, so
            # buildGoModule alone cannot satisfy gopyter's go.mod.
            pkgs.buildGoLatestModule
            {
              pname = "gopyter";
              version = rev;
              src = ./.;

              # gopyter's workspace is offline at runtime, and vendoring the
              # module graph into the store would add nothing to the binary, so
              # the hash is the one nixpkgs reports for `go mod vendor`.
              vendorHash = "sha256-3eZ+D1+DhbWZcm2CZD9Yn2p28Q0eIAsMZKKTQv41Imk=";

              env.CGO_ENABLED = "0";

              # main.version and main.commit feed fang's --version output, the
              # same variables .goreleaser.yaml sets.
              ldflags = [
                "-s"
                "-w"
                "-X main.version=${rev}"
                "-X main.commit=${rev}"
              ];

              # The kernel tests compile and run real programs against a
              # temporary Go workspace; they are slow and need the toolchain
              # the build already provides. CI runs them on every push.
              doCheck = false;

              meta = {
                description = "A Jupyter-style notebook for Go that runs in the terminal";
                homepage = "https://github.com/mark3labs/gopyter";
                license = pkgs.lib.licenses.mit;
                mainProgram = "gopyter";
                platforms = systems;
              };
            };

          default = self.packages.${system}.gopyter;
        }
      );

      checks = forAllSystem (system: {
        gopyter = self.packages.${system}.gopyter;
      });
    };
}
