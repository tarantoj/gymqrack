{
  config,
  lib,
  pkgs,
  ...
}:
{
  # mkDefault: when composed into the root environment the root name wins;
  # standalone (entering apple/) this value applies.
  name = lib.mkDefault "vivagym-apple";

  # Apple Watch app dev environment. This module is composed into the root
  # environment via devenv.yaml `imports`; entering apple/ activates it alone.
  # xcodegen and mas are darwin-only (`meta.platforms` in nixpkgs), so the
  # whole toolchain is gated on the host OS; CI's ubuntu leg must not build it.
  packages = lib.mkIf pkgs.stdenv.isDarwin (
    with pkgs;
    [
      xcodegen # generate the Xcode project from project.yml (works without Xcode)
      mas # CLI for the Mac App Store (download/install Xcode)
    ]
  );

  # When a full Xcode is present (device builds/deploys), prefer it over the
  # nix apple-sdk shim that devenv otherwise points DEVELOPER_DIR at, and point
  # the linker at Xcode's toolchain so the final link step doesn't pick up the
  # nix cctools ld from PATH (which rejects Xcode's -Xlinker flag stream). CI
  # and Xcode-less shells keep the nix SDK.
  env = lib.mkIf (pkgs.stdenv.isDarwin && builtins.pathExists /Applications/Xcode.app) {
    DEVELOPER_DIR = "/Applications/Xcode.app/Contents/Developer";
    LD = "/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/ld";
  };

  languages.swift.enable = lib.mkIf pkgs.stdenv.isDarwin true;

  scripts = {
    generate.exec = ''
      cd ${config.git.root}/apple
      xcodegen generate
    '';
    open.exec = ''
      open ${config.git.root}/apple/VivaGym.xcodeproj
    '';
    build.exec = ''
      cd ${config.git.root}/apple
      xcodegen generate
      xcodebuild -project VivaGym.xcodeproj \
        -scheme VivaGymWatch \
        -destination 'generic/platform=watchOS' \
        build
    '';
    build-ios.exec = ''
      cd ${config.git.root}/apple
      xcodegen generate
      xcodebuild -project VivaGym.xcodeproj \
        -scheme VivaGymWatchCompanion \
        -destination 'platform=iOS Simulator,name=iPhone 17' \
        build
    '';
    test.exec = ''
      cd ${config.git.root}/apple
      xcodegen generate
      xcodebuild -project VivaGym.xcodeproj \
        -scheme VivaGymKitTests \
        -destination 'platform=iOS Simulator,name=iPhone 17' \
        test
    '';
  };

  enterShell = ''
    echo "VivaGym Apple dev shell"
    echo "  generate : xcodegen generate        (works without Xcode)"
    echo "  build    : build VivaGymWatch         (requires Xcode + watchOS SDK)"
    echo "  build-ios: build VivaGymWatchCompanion (requires Xcode)"
    echo "  test     : run VivaGymKitTests"
    echo "Note: install full Xcode from the App Store to build; xcodebuild is"
    echo "only available then. xcodegen needs no Xcode."
  '';
}
