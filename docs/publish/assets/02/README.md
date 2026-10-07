# 02편 이미지 제작 기록

- 대표 이미지 1장은 한글·영문 공용이며 도식 3종은 한글·영문을 각각 제작했습니다.
- `01-hero.png`: 내장 image_gen 도구로 생성한 개념 일러스트. 실제 보드 사진이나 실제 칩 내부 구조를 나타내지 않습니다.
- `02-secure-element-roles.png`, `02-secure-element-roles-en.png`: Ledger와 Trezor Safe 3·5의 공식 설명을 바탕으로 직접 작성했습니다.
- `03-trustzone-signing-flow.png`, `03-trustzone-signing-flow-en.png`: 목표 설계를 단순화한 흐름도이며 보안 구역 구현 검증 결과가 아닙니다.
- `04-approval-and-display.png`, `04-approval-and-display-en.png`: 키 보호·버튼 입력·표시 내용의 일치를 구분합니다. A·B는 실제 결제 기록이 아닙니다.
- 도식은 `build-diagrams.py`로 재현합니다. 생성 뒤 두 언어의 글자와 화살표를 시각적으로 확인했습니다.
- 제작일: 2026-10-06.

## 출처

- [Ledger 공식 설명](https://www.ledger.com/academy/security/the-secure-element-whistanding-security-attacks)
- [Trezor 공식 설명](https://trezor.io/learn/security-privacy/how-trezor-keeps-you-safe/secure-elements-in-trezor-safe-devices)
- [Arm TrustZone 설명](https://www.arm.com/technologies/trustzone-for-cortex-m)

## 대표 이미지 최종 프롬프트

```text
Use case: stylized-concept
Asset type: Shared text-free hero illustration for Korean and English technical Medium article about hardware-wallet private-key protection.
Primary request: A clear editorial conceptual illustration comparing two ways to protect a cryptographic key. Left: a dedicated small secure-element chip, shown as a solid compact component containing a simple golden key icon. Right: a larger microcontroller chip shown in a conceptual cutaway with two physically separated colored regions; the teal region contains the same simple key icon and the gray region contains a wireless signal icon. Show both on a tidy dark PCB workbench, without screens or product branding.
Style/medium: Refined isometric 3D technical illustration, warm off-white background, muted graphite components, teal protection boundaries and gold key accents. Landscape with generous clean margins and balanced equal prominence. Thin PCB traces suggest connectivity, without any key icon or key material leaving either component.
Constraints: Conceptual illustration only, not a real board photograph. No words, letters, numbers, hashes, addresses, logos, watermark, shield checkmarks, claims of equal security, people or money. Do not depict TrustZone as a separate external chip. Do not draw the comparison as a contest winner.
```

