import UIKit
import React
import React_RCTAppDelegate
import ReactAppDependencyProvider

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
  private let reactNativeDelegate = ReactNativeDelegate()
  lazy var reactNativeFactory: RCTReactNativeFactory = {
    reactNativeDelegate.dependencyProvider = RCTAppDependencyProvider()
    return RCTReactNativeFactory(delegate: reactNativeDelegate)
  }()

  private(set) var launchOptions: [UIApplication.LaunchOptionsKey: Any]?

  func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    self.launchOptions = launchOptions
    return true
  }
}

final class SceneDelegate: UIResponder, UIWindowSceneDelegate {
  var window: UIWindow?

  func scene(
    _ scene: UIScene,
    willConnectTo session: UISceneSession,
    options connectionOptions: UIScene.ConnectionOptions
  ) {
    guard let windowScene = scene as? UIWindowScene,
          let app = UIApplication.shared.delegate as? AppDelegate else { return }
    let window = UIWindow(windowScene: windowScene)
    self.window = window
    let launchOptions = app.launchOptions
    app.reactNativeFactory.startReactNative(
      withModuleName: "NU54Kiosk",
      in: window,
      launchOptions: launchOptions
    )
  }
}

class ReactNativeDelegate: RCTDefaultReactNativeFactoryDelegate {
  override func sourceURL(for bridge: RCTBridge) -> URL? {
    self.bundleURL()
  }

  override func bundleURL() -> URL? {
#if DEBUG
    RCTBundleURLProvider.sharedSettings().jsBundleURL(forBundleRoot: "index")
#else
    Bundle.main.url(forResource: "main", withExtension: "jsbundle")
#endif
  }
}
