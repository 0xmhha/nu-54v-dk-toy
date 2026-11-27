# W04 Android permission recovery

Use a dedicated or assigned Android 14 arm64 emulator with an English system UI, Bluetooth on, and Android SDK, Java and repository-pinned Node available. The installed app starts in Korean.

```sh
scripts/android/w04-smoke/run.sh emulator-5598
```

The script builds an isolated custom variant using a throwaway signer. It installs the W04 package and a disposable clipboard helper, drives the ordinary personal-board connection screen and removes the two packages at exit. It fails before installation if either package already exists. The runner enters a public synthetic registration document, device identity and independently supplied key fingerprint through the visible fields. It denies Nearby devices in the real Android prompt, confirms that both permissions remain disabled and that registration cannot start, then uses the app's Settings action. The runner compares the three accessibility field values before requesting permission and after returning from Settings. Value checks are separate from deciding which controls are visible enough to tap. A failed comparison logs only its stage and count. The runner then taps the visible retry action for the same search. Success requires a completion message from the native Bluetooth scan.

The OS run exercises the permission prompt, Android Settings, return to the populated enrollment and the native radio scanner. The fixture provides no GATT board. The companion `w04W06OwnerUi` test checks that permission denial neither saves nor opens a session, and that the same enrollment invokes registration once after permission is granted and the user selects its candidate. The `ownerSession` regression performs an actual P01 C host commit once and verifies restart after a lost reply. These are separate observations. Android radio-to-board enrollment and physical board approval still require a board or a GATT counterpart.

The Android 14 emulator run passed on 2026-11-21. It confirmed denial in the OS prompt, both permissions granted in Settings, the same three enrollment values and completion of the native scan after one explicit retry. The aggregate digest is `sha256:e092b06214e50c627c8904ee9446225c270af03cf78fada38902bb4321f76fff`. P02 type checks and all 302 tests passed.
