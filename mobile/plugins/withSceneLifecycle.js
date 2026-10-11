// Expo config plugin: adopt the UIScene life cycle, which iOS 27 requires at
// launch (UIKit traps otherwise). Expo ships ExpoAppSceneDelegate, but the
// prebuild template still starts React Native from the AppDelegate, so this
// declares a scene manifest, adds a SceneDelegate and leaves the AppDelegate
// to only create the React Native factory.
const fs = require('fs');
const path = require('path');
const { IOSConfig, withAppDelegate, withDangerousMod, withInfoPlist, withXcodeProject } = require('expo/config-plugins');

const SCENE_DELEGATE = `internal import Expo
import UIKit

class SceneDelegate: ExpoAppSceneDelegate {}
`;

const START_IN_APP_DELEGATE = /#if os\(iOS\) \|\| os\(tvOS\)\n\s*window = UIWindow\(frame: UIScreen\.main\.bounds\)\n\s*factory\.startReactNative\([\s\S]*?\)\n#endif\n\n?/;

module.exports = function withSceneLifecycle(config) {
  config = withInfoPlist(config, (c) => {
    c.modResults.UIApplicationSceneManifest = {
      UIApplicationSupportsMultipleScenes: false,
      UISceneConfigurations: {
        UIWindowSceneSessionRoleApplication: [
          { UISceneConfigurationName: 'Default Configuration', UISceneDelegateClassName: '$(PRODUCT_MODULE_NAME).SceneDelegate' },
        ],
      },
    };
    return c;
  });
  config = withAppDelegate(config, (c) => {
    if (c.modResults.language !== 'swift') throw new Error('withSceneLifecycle: only the Swift AppDelegate is supported');
    let src = c.modResults.contents;
    if (!src.includes('ExpoReactNativeFactoryProvider')) {
      src = src.replace('class AppDelegate: ExpoAppDelegate {', 'class AppDelegate: ExpoAppDelegate, ExpoReactNativeFactoryProvider {');
      if (!src.includes('ExpoReactNativeFactoryProvider')) throw new Error('withSceneLifecycle: AppDelegate class declaration not found');
    }
    if (START_IN_APP_DELEGATE.test(src)) {
      src = src.replace(START_IN_APP_DELEGATE, ''); // the scene delegate creates the window and starts React Native
    } else if (src.includes('startReactNative')) {
      throw new Error('withSceneLifecycle: unexpected startReactNative block in AppDelegate');
    }
    c.modResults.contents = src;
    return c;
  });
  config = withDangerousMod(config, [
    'ios',
    (c) => {
      fs.writeFileSync(path.join(c.modRequest.platformProjectRoot, c.modRequest.projectName, 'SceneDelegate.swift'), SCENE_DELEGATE);
      return c;
    },
  ]);
  config = withXcodeProject(config, (c) => {
    const filepath = `${c.modRequest.projectName}/SceneDelegate.swift`;
    if (!c.modResults.hasFile(filepath)) {
      IOSConfig.XcodeUtils.addBuildSourceFileToGroup({ filepath, groupName: c.modRequest.projectName, project: c.modResults });
    }
    return c;
  });
  return config;
};
