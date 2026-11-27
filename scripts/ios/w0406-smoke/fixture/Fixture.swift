import UIKit
import Security
@main class AppDelegate: UIResponder, UIApplicationDelegate {
 var window: UIWindow?
 func application(_ application: UIApplication, didFinishLaunchingWithOptions options: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
  let query: [String:Any] = [kSecClass as String:kSecClassGenericPassword,kSecAttrService as String:"nu54.personal.profile.v2",kSecAttrAccount as String:"phone-local",kSecAttrSynchronizable as String:false]
  let board = String(repeating:"a",count:64)
  let address = "0x" + String(repeating:"1",count:40)
  let scope = board + "/1/stable/0x2bdf79b3d3cc49f9e6638ff81f3bb85065c79945a8fe4556cd0ff47bbfc02490//0/0/native"
  let profile: [String:Any] = ["version":2,"exercised":false,"mode":"personal","verified":["boardFingerprint":board,"address":address,"network":"8283","generation":1],"enrollment":["version":2,"device":board,"boardSPKI":String(repeating:"b",count:64),"ca":"MA==","certificateChain":["MA=="]],"walletRead":["selection":["id":"stable","account":0,"hidden":false],"endpoints":[:],"contacts":[],"observations":[["scope":scope,"address":address,"amount":"1250000","unit":"TEST","decimals":6,"point":"Finalized block 45","time":1700000000000]]]]
  let fixture = String(data:try! JSONSerialization.data(withJSONObject:profile),encoding:.utf8)!
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
