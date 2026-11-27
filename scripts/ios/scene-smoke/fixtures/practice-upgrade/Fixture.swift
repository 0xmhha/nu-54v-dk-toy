import UIKit
import Security
@main class AppDelegate: UIResponder, UIApplicationDelegate {
 var window: UIWindow?
 func application(_ application: UIApplication, didFinishLaunchingWithOptions options: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
  let query: [String:Any] = [kSecClass as String:kSecClassGenericPassword,kSecAttrService as String:"nu54.personal.profile.v2",kSecAttrAccount as String:"phone-local",kSecAttrSynchronizable as String:false]
  let fixture = "{\"version\":2,\"exercised\":false,\"mode\":\"personal\",\"verified\":{\"boardFingerprint\":\"" + String(repeating:"a",count:64) + "\",\"address\":\"0x" + String(repeating:"1",count:40) + "\",\"network\":\"8283\",\"generation\":1}}"
  let data = fixture.data(using:.utf8)!
  var result = SecItemUpdate(query as CFDictionary, [kSecValueData as String:data] as CFDictionary)
  if result == errSecItemNotFound { var add=query; add[kSecValueData as String]=data; add[kSecAttrAccessible as String]=kSecAttrAccessibleWhenUnlockedThisDeviceOnly; result=SecItemAdd(add as CFDictionary,nil) }
  NSLog("fixture keychain status %d", result); UserDefaults.standard.set(result == errSecSuccess, forKey:"fixtureWritten"); return true
 }
}

class SceneDelegate: UIResponder, UIWindowSceneDelegate {
 var window: UIWindow?
 func scene(_ scene: UIScene, willConnectTo session: UISceneSession, options: UIScene.ConnectionOptions) {
  guard let scene = scene as? UIWindowScene else { return }
  window = UIWindow(windowScene: scene); let vc = UIViewController()
  vc.view.backgroundColor = UserDefaults.standard.bool(forKey:"fixtureWritten") ? .green : .red
  window?.rootViewController = vc; window?.makeKeyAndVisible()
 }
}
