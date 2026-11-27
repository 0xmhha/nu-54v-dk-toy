import UIKit
import UserNotifications
import React
import React_RCTAppDelegate
import ReactAppDependencyProvider

private func subscriptionRoute(_ response: UNNotificationResponse) -> URL? {
  let ready = "nu54-order-"
  if response.actionIdentifier == UNNotificationDefaultActionIdentifier,
     response.notification.request.identifier.hasPrefix(ready),
     let order = UUID(uuidString: String(response.notification.request.identifier.dropFirst(ready.count))) {
    return URL(string: "nu54://order/ready/\(order.uuidString.lowercased())")
  }
  let prefix = "nu54-subscription-"
  let identifier = response.notification.request.identifier
  guard response.actionIdentifier == UNNotificationDefaultActionIdentifier,
        identifier.hasPrefix(prefix),
        let reference = UUID(uuidString: String(identifier.dropFirst(prefix.count))) else { return nil }
  return URL(string: "nu54://subscriptions/\(reference.uuidString.lowercased())")
}

@main
final class AppDelegate: UIResponder, UIApplicationDelegate, UNUserNotificationCenterDelegate {
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
    UNUserNotificationCenter.current().delegate = self
    return true
  }

  // An order can become ready while the app is open; the notice still shows.
  func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification,
                              withCompletionHandler completion: @escaping (UNNotificationPresentationOptions) -> Void) {
    completion(notification.request.identifier.hasPrefix("nu54-order-") ? [.banner, .sound] : [])
  }

  func userNotificationCenter(_ center: UNUserNotificationCenter,
                              didReceive response: UNNotificationResponse,
                              withCompletionHandler completion: @escaping () -> Void) {
    DispatchQueue.main.async {
      if let route = subscriptionRoute(response) {
        _ = RCTLinkingManager.application(UIApplication.shared, open: route, options: [:])
      }
      completion()
    }
  }
}

final class SceneDelegate: UIResponder, UIWindowSceneDelegate {
  var window: UIWindow?
  private var homeProtected = false
  private var privacyCover: UIView?
  private var protectionObserver: NSObjectProtocol?

  private func updateCover() {
    guard let window = window else { return }
    if homeProtected && window.windowScene?.activationState != .foregroundActive {
      if privacyCover == nil {
        let cover = UIView(frame: window.bounds)
        cover.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        cover.backgroundColor = .systemBackground
        cover.isAccessibilityElement = true
        cover.accessibilityLabel = NSLocalizedString("Wallet content hidden", comment: "")
        cover.accessibilityViewIsModal = true
        window.addSubview(cover)
        privacyCover = cover
      }
    } else {
      privacyCover?.removeFromSuperview()
      privacyCover = nil
    }
  }

  func sceneWillResignActive(_ scene: UIScene) {
    guard homeProtected, let window = window else { return }
    if privacyCover == nil {
      let cover = UIView(frame: window.bounds)
      cover.autoresizingMask = [.flexibleWidth, .flexibleHeight]
      cover.backgroundColor = .systemBackground
      cover.isAccessibilityElement = true
      cover.accessibilityLabel = "Wallet content hidden"
      cover.accessibilityViewIsModal = true
      window.addSubview(cover)
      privacyCover = cover
    }
  }

  func sceneDidBecomeActive(_ scene: UIScene) { updateCover() }
  func sceneDidDisconnect(_ scene: UIScene) {
    if let observer = protectionObserver { NotificationCenter.default.removeObserver(observer) }
    protectionObserver = nil
    privacyCover = nil
  }

  func scene(
    _ scene: UIScene,
    willConnectTo session: UISceneSession,
    options connectionOptions: UIScene.ConnectionOptions
  ) {
    guard let windowScene = scene as? UIWindowScene,
          let app = UIApplication.shared.delegate as? AppDelegate else { return }
    let window = UIWindow(windowScene: windowScene)
    self.window = window
    protectionObserver = NotificationCenter.default.addObserver(forName: Notification.Name("NU54HomeProtection"), object: nil, queue: .main) { [weak self] note in
      self?.homeProtected = note.userInfo?["enabled"] as? Bool ?? false
      self?.updateCover()
    }
    // React Native Linking.getInitialURL reads the bridge launch options.
    var launchOptions = app.launchOptions ?? [:]
    if let response = connectionOptions.notificationResponse, let route = subscriptionRoute(response) {
      launchOptions[.url] = route
    } else if let context = connectionOptions.urlContexts.first {
      launchOptions[.url] = context.url
      launchOptions[.sourceApplication] = context.options.sourceApplication
      launchOptions[.annotation] = context.options.annotation
    }
    app.reactNativeFactory.startReactNative(
      withModuleName: "NU54Renter",
      in: window,
      launchOptions: launchOptions
    )
  }

  func scene(_ scene: UIScene, openURLContexts contexts: Set<UIOpenURLContext>) {
    for context in contexts {
      var options: [UIApplication.OpenURLOptionsKey: Any] = [
        .openInPlace: context.options.openInPlace
      ]
      options[.sourceApplication] = context.options.sourceApplication
      options[.annotation] = context.options.annotation
      _ = RCTLinkingManager.application(UIApplication.shared, open: context.url, options: options)
    }
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
