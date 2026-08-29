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
  packages = with pkgs; [
    xcodegen # generate the Xcode project from project.yml (works without Xcode)
  ];

  languages.swift.enable = true;

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
