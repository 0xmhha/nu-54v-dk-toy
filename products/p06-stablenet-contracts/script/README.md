# 배포 스크립트

Foundry 배포 스크립트(`*.s.sol`)를 둔다. 첫 스크립트는 WBS2-P06-02(8283 배포와 소프트웨어
서명 정산)와 함께 추가한다. 개인키는 저장소에 두지 않고 `cast wallet import`로 만든
암호화 keystore 파일을 secretRef(`NU54_DEPLOYER_KEYSTORE` 경로와 `NU54_DEPLOYER_PASSWORD_ENV`가 가리키는 암호 환경 변수)로 연다 [N32].
