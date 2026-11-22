# Pretendard

Pretendard 1.3.9 by Kil Hyung-jin, licensed under the SIL Open Font License 1.1 (`LICENSE.txt`).
Source: the official release asset `Pretendard-1.3.9.zip` from github.com/orioncactus/pretendard,
sha256 `04be351a74d6bf7d60c480a3087e51d185485d35a52023142af1df19eb8c428a`.

The Android app bundles three static weights from `public/static/` without changes:
Regular (400), Bold (700) and ExtraBold (800), as `android/app/src/main/res/font/pretendard_*.otf`.
`MainApplication` registers them as the `Pretendard` family so `fontWeight` picks the right file.
The iOS app does not bundle the font yet and falls back to the system font.
