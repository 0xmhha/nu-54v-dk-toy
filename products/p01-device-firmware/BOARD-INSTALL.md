# 새 보드 설치 증거: 커스텀 시험 경로

W03은 설치 이미지 사전 검증과 설치된 기기의 관측을 구분한다. 공식 보드 키는 아직 등록되지 않아 공식 경로를 열지 않는다. 아래 절차는 별도 경로에서 신뢰한 커스텀 릴리스 키, 독립 설치자 키와 부트키 지문을 가진 제작자용이다. 앱은 사용자 개인키나 seed를 받지 않는다.

1. P10의 서명된 릴리스 manifest, detached 서명, 공개키, 모든 참조 파일과 신뢰 정책을 준비한다. `p01-image`는 MCUboot 서명 이미지, `p01-provenance`는 그 이미지의 P01 빌드 manifest다. 정책에는 릴리스 서명자와 다른 Ed25519 설치자 공개키를 `installers.custom`의 `p01-image` 권한으로 등록한다. 설치자 SPKI SHA-256, 릴리스 manifest SHA-256, 대상 보드 식별값은 bundle과 별개 신뢰 경로에서 확인한다.
2. 컴퓨터에서 예를 들어 아래를 실행한다. 독립 부트키 지문과 해당 기기에서 허용할 최소 보안 카운터를 지정한다. 기존 출력 파일은 덮어쓰지 않는다.

   ```sh
   python3 products/p01-device-firmware/scripts/verify_board_release.py \
     --manifest RELEASE --signature SIGNATURE --public-key RELEASE_PUBLIC \
     --trust INDEPENDENT_POLICY --channel custom --boot-key-sha256 BOOT_KEY_SHA256 \
     --minimum-counter MIN_COUNTER --boot-public BOOT_PUBLIC \
     --installer-key INSTALLER_PUBLIC --setup-out SETUP_JSON
   ```

   성공 결과는 **preflight-only**다. P10의 릴리스 서명과 산출물 증거, 모델과 버전, 이미지 전체 해시, P01 빌드 기록의 보안 카운터를 검사한다. MCUboot 헤더·이미지·보호 TLV를 합친 구간의 SHA-256도 별도로 계산한다. 위 명령으로 설치 요청서를 만들거나 5단계에서 확인서를 만들 때는 `--boot-public`이 필수다. CLI가 독립 지문에 일치하는 부트 공개키로 이미지 서명을 검증하고 모델·버전·카운터와 보호 구간 측정값을 대조한 뒤에만 파일을 발급한다. 실제 설치·부팅 결과는 4단계에서 기기로부터 따로 관측한다. [설치 도구](scripts/verify_board_release.py) · [MCUboot 이미지 검증기](../p05-operations-backoffice/cmd/verify-firmware)
3. W03에서 시스템 문서 선택기로 setup JSON을 열고, 별도 신뢰 경로에서 받은 `signer`, `candidate`, `device`의 SHA-256 hex 세 필드를 갖는 독립 trust JSON을 따로 선택한다. 표시된 값이 별도 기록과 일치하는지 사용자가 비교·확인한다. 앱이 생성한 단회 challenge는 보호 저장소에 남아 중단 후 같은 요청을 재개한다. 이 challenge를 설치 관측자에게 전달한다.
4. 설치자는 실제 기기의 부트 측정·카운터·보드 ID·등록 세대와 challenge를 관측해 `nu54-board-observation-v1` 보고서를 만들고 P10 `nu54-installation-v1` 보고서 참조를 통해 독립 설치자 키로 서명한다. 현재 이 저장소에는 **실물 기기에서 이 관측 보고서를 자동 발급하는 도구가 없다**. 테스트에서 쓰는 보고서는 합성이며 실제 설치 증거가 아니다.
5. 신뢰된 독립 설치자는 같은 P10 검증 옵션에 `--installation RECORD RECORD_SIGNATURE INSTALLER_PUBLIC --attester-private INSTALLER_PRIVATE --receipt-out RECEIPT_JSON`을 더해 앱 전달용 서명 확인서를 발급한다. 발급기는 P10 보고서 서명과 범위, report 해시, 후보·모델·부트키·측정·카운터·시간을 다시 확인한다. 개인키는 컴퓨터의 독립 설치자 경계에서만 사용한다.
6. W03에서 확인서를 검증하면 같은 보드 등록으로 이동한다. 앱은 사용자가 독립 확인한 설치자 SPKI 지문과 릴리스 해시, 보드 ID에 대해 서명과 challenge·유효기간을 검사한다. W04에서 인증된 BLE 세션으로 보드 ID와 등록 세대를 다시 확인한다. 시간 만료 또는 다른 보드·후보·challenge는 기존 요청을 취소하고 독립 확인부터 다시 시작한다. 보드 부팅 후 실제 측정 재확인 및 운영 신뢰 체인의 실기 검증은 출시 전 별도 인수다.

합성 벡터는 `python3 -m unittest discover -s products/p01-device-firmware/scripts -p test_verify_board_release.py`로 실행한다. 테스트의 `observed`는 서명·연결 규칙을 검증하는 합성 입력으로서 실제 기기 관측이 아니다.

앱은 시스템 문서 선택으로 `.json`을 읽고 8 KiB 초과와 다른 확장자를 거절한다. 직접 입력은 보조 경로다. 파일만으로 신뢰를 부여하거나 등록하지 않으며, 독립 설치자 지문·릴리스 해시·보드 ID를 별도 확인한 뒤 서명·challenge를 검증한다. 단회 QR은 후속 대안이다.
