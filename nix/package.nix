{
  lib,
  buildGoModule,
  version,
}:
buildGoModule {
  pname = "seshagy";
  inherit version;

  src = lib.cleanSource ../.;
  vendorHash = lib.fakeHash;

  subPackages = ["cmd/seshagy"];
  env.CGO_ENABLED = 0;
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];

  doCheck = true;
  checkPhase = ''
    runHook preCheck
    go test ./...
    runHook postCheck
  '';

  doInstallCheck = true;
  installCheckPhase = ''
    runHook preInstallCheck
    test "$($out/bin/seshagy --version)" = "${version}"
    runHook postInstallCheck
  '';

  meta = {
    description = "Agent-aware terminal dashboard for tmux and herdr";
    homepage = "https://github.com/lmilojevicc/seshagy";
    license = lib.licenses.mit;
    mainProgram = "seshagy";
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
      "aarch64-darwin"
    ];
  };
}
