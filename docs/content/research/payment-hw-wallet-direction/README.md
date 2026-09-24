# 결제 시장이 아니라 서명 신뢰를 겨냥하십시오

조사일: 2026-09-23. 입력 자료: [`notes/`](notes/) 5개 노트. 모든 수치에는 출처 URL과 발표·보도 시점을 붙였습니다. **[미검증]**은 2차 출처나 검색 요약에서만 확인한 수치, **[상충]**은 출처끼리 값이 다른 수치를 뜻합니다.

**결론부터 말씀드리면, "금융 인프라가 부족한 국가에서 블록체인 결제가 유용하다"는 명제는 근거가 약하고, "앱 기반 크립토 결제의 서명 보안이 허술하다"는 명제는 근거가 강합니다.** 저소득·중소득국의 스테이블코인 수요는 실재하지만 그 내용은 달러 저축·환헤지·B2B 수입대금·송금이며, 매장 결제(POS)는 가장 작고 가장 입증되지 않은 용도입니다. 맥킨지·Artemis 추정으로 연 약 35조 달러 스테이블코인 거래 중 실제 결제는 약 3,900억 달러(약 1%)이고, 그중 카드 연동 소비는 약 45억 달러에 그칩니다. 국내 소액 결제는 모바일 머니(월간 활성 계정 5.93억, 2025년 거래액 2조 달러 초과)가 이미 해결하고 있고, 은행 계좌가 없는 성인 중 약 3.7억 명은 스마트폰 없이 피처폰만 가지고 있어 앱 기반 크립토 지갑이 닿지 않습니다. 반면 카드사·PG사는 2024~2026년에 스테이블코인을 정산·카드 뒷단에 빠르게 흡수했지만 거의 모든 제품이 수탁형(custodial)이거나 사전 승인(allowance) 방식이어서, **하드웨어 키가 매장 결제 한 건 한 건을 직접 서명하는 제품은 시장에 없습니다.** 개인 지갑 탈취는 2025년 15.8만 건, 7.13억 달러였고, 사고 원인은 칩 파괴가 아니라 블라인드 서명, 주소 바꿔치기, 시드 피싱, 펌웨어 난수 버그, 고객 정보 유출이었습니다. 따라서 3인 12주 PoC는 "개도국 결제 수요를 입증"하는 방향이 아니라 **"기존 하드웨어 월렛 사고에서 도출한 보안 요구사항을 결제 흐름에 적용해, 판매자 인증이 된 결제 요청을 기기 화면에서 해석·확인하고 서명하는 기기"를 테스트넷에서 보여주는 방향**으로 정의하는 것이 근거와 맞습니다. nRF54L15는 PSA Certified Level 1만 보유하고 secp256k1 하드웨어 가속이 확인되지 않았으므로, 시드 보호에는 외부 보안 칩(Secure Element, SE)이 필요합니다.

---

## 1. 개도국의 스테이블코인 수요는 저축과 송금이지 매장 결제가 아닙니다

### 근거가 뒷받침하지 않는 것부터

방향 재설정의 전제인 "금융 인프라가 부족한 국가에서 블록체인 결제가 유용하다"는 주장을 그대로 발표하면 반박당하기 쉽습니다. 가장 강한 반례는 엘살바도르입니다. 정부가 비트코인을 법정화폐로 지정하고, 지갑(Chivo) 가입자에게 30달러를 지급하고, 수수료를 없앴는데도 IMF 분석에서 **기업의 97.75%가 비트코인 매출이 한 건도 없었고**, 비트코인 결제는 매출의 4.9%, 크립토 지갑을 거친 송금은 약 1.75%였습니다. 이용자 대부분은 30달러 보너스만 받고 추가 거래를 하지 않았고(60%), IMF는 "금융 포용에 진전이 없었다"고 결론 내렸습니다 ([IMF Country Report 25/68, 2025-03 발간, 2022년 데이터](https://www.imf.org/-/media/files/publications/cr/2025/english/1slvea2025003-print-pdf.pdf)). 엘살바도르는 이미 달러를 쓰고 스마트폰 보급률이 68%인 나라였으므로, 기술과 보조금만으로는 결제 습관이 생기지 않는다는 강한 증거입니다. 2025-01-29 IMF 14억 달러 지원 조건으로 비트코인 법이 개정되어 수용 의무와 세금 납부 옵션이 사라졌습니다. 다만 "더 이상 법정화폐가 아니다"와 "자발적 법정화폐로 남았다"는 보도가 엇갈립니다 **[상충]** ([Forbes, 2025-02-28](https://www.forbes.com/sites/digital-assets/2025/02/28/el-salvadors-bitcoin-law-changes-to-secure-imf-funding/); [Global Finance](https://gfmag.com/economics-policy-regulation/el-salvador-drops-bitcoin-legal-tender/)).

용도별 규모도 같은 방향을 가리킵니다. 맥킨지·Artemis는 연환산 약 35조 달러 스테이블코인 거래량 중 **실제 최종 사용자 결제는 약 3,900억 달러(약 1%)**이고, 그 구성은 B2B 약 2,260억 달러(약 58%), 급여·송금 약 900억 달러, 카드 연동 소비 약 45억 달러이며, 이는 전 세계 결제의 약 0.02%라고 추정했습니다. 기준은 2025년 12월 활동의 연환산이고, 보고서 본문은 타임아웃으로 직접 읽지 못해 2차 요약에 의존했으므로 **[미검증]**으로 표시합니다 ([McKinsey](https://www.mckinsey.com/industries/financial-services/our-insights/stablecoins-in-payments-what-the-raw-transaction-numbers-miss); [Artemis PDF](https://reports.artemisanalytics.com/stablecoins/artemis-stablecoin-payments-from-the-ground-up-2025.pdf)). 카드 소비 45억 달러는 "실제 결제" 3,900억 달러의 약 1.2%에 불과하므로, **매장 결제는 스테이블코인 용도 중 가장 작고 가장 입증되지 않은 영역**입니다.

Chainalysis 순위를 "개도국 사람들이 크립토로 결제한다"는 근거로 쓰는 것도 피해야 합니다. 2025년 지수 상위권은 인도·미국·파키스탄·베트남·브라질·나이지리아 순이지만, 이 지수는 중앙화 거래소 유입액, 1만 달러 미만 소매 이체, 100만 달러 초과 기관 이체 등 **온체인 수취 가치**를 PPP 1인당 GDP로 가중한 것이고, 2025년판부터는 기관 하위 지수가 추가되었습니다 ([Chainalysis, 2025-09-02](https://www.chainalysis.com/blog/2025-global-crypto-adoption-index/)). 사하라 이남 아프리카의 1만 달러 미만 소매 이체 비중은 가치 기준 8%(전 세계 6%)에 불과하고, 같은 보고서 요약에는 나이지리아 법정화폐 매수의 89%가 BTC, USDT는 7%로 나오는데, 이는 "스테이블코인 주도" 서사와 맞지 않아 원문 확인이 필요합니다 **[미검증]** ([Chainalysis SSA, 2025-09-10](https://www.chainalysis.com/blog/subsaharan-africa-crypto-adoption-2025/)). a16z는 크립토 보유자 약 7.16억 명 중 **실제 활동 사용자는 4천만~7천만 명**으로 추정했습니다 ([a16z State of Crypto, 2025-10-22](https://a16zcrypto.com/posts/article/state-of-crypto-report-2025/)).

### 모바일 머니가 이미 국내 결제를 해결했습니다

GSMA 2026년판(2025년 데이터)에 따르면 모바일 머니는 **등록 계정 23억, 30일 활성 계정 5.93억, 연간 거래액 2조 달러 초과**입니다 ([GSMA, 2026-03-24](https://www.gsma.com/newsroom/press-release/mobile-money-accounted-for-2-trillion-in-transactions-in-2025-doubling-since-2021-as-active-accounts-continue-to-grow/)). 활성 사용자만 비교해도 크립토 활성 사용자(4천만~7천만)의 약 10배입니다. 두 집단은 겹치고 측정 방식이 달라 정밀 비교는 아닙니다. 케냐 M-Pesa는 소액 P2P가 무료이고 1,000실링 송금 수수료가 13실링(약 1.3%)이며, 타 통신사 지갑으로도 같은 요금이 적용됩니다 ([Safaricom 요금표](https://www.safaricom.co.ke/main-mpesa/m-pesa-for-you/tariffs-limits/consumer-tariffs-limits)). 나이지리아 NIP, 브라질 Pix, 인도 UPI도 국내 즉시 결제를 저렴하게 처리합니다. 스테이블코인 지갑이 여기에 더하는 것은 "더 싼 국내 결제망"이 아니라 **달러 표시 자산과 국경 간 이동**입니다. 모바일 머니에도 약점은 있어서 등록 계정의 약 75%가 월간 비활성입니다.

### 3.7억 명은 스마트폰이 없습니다

세계은행 Global Findex 2025(2024년 조사, 2025-07 발표)에 따르면 은행 계좌가 없는 성인 13억 명 중 **약 9억 명이 휴대폰을 가지고 있지만 스마트폰 보유자는 5.3억 명**입니다 ([World Bank Global Findex 2025](https://www.worldbank.org/en/publication/globalfindex); [BIIA 요약(2차)](https://www.biia.com/financial-inclusion-at-record-high-but-1-3-billion-still-unbanked-world-bank-global-findex-2025-report/)). 차이인 약 3.7억 명은 USSD·SMS 같은 피처폰 방식으로만 닿을 수 있고, 휴대폰이 아예 없는 약 4억 명은 그마저도 어렵습니다(두 수치 모두 Findex 수치에서 산술로 도출). 사하라 이남 아프리카는 스마트폰이 전체 회선의 절반 미만이고 모바일 인터넷 미사용 격차가 약 53%입니다 **[미검증: GSMA PDF 원문 미확인]** ([GSMA M4D](https://www.gsma.com/solutions-and-impact/connectivity-for-good/mobile-for-development/blog/the-state-of-mobile-internet-connectivity-in-sub-saharan-africa/)). BLE 하드웨어 월렛은 보통 스마트폰 앱을 호스트로 쓰므로, **고객 스마트폰을 전제하면 이 3.7억 명에게 닿지 않는다는 점을 발표에서 스스로 밝혀야 합니다.**

### 그래도 실재하는 수요

수요가 없는 것은 아닙니다. 사하라 이남 아프리카는 2024년판에서 거래량의 약 43%가 스테이블코인이었고, 2024년 7월 비르 화폐가 약 30% 평가절하된 뒤 에티오피아의 소매 규모 스테이블코인 이체가 전년 대비 180% 늘었습니다 ([Chainalysis SSA, 2024-10-02](https://www.chainalysis.com/blog/subsaharan-africa-crypto-adoption-2024/)). 43%는 2024년판 수치이므로 2025년 수치로 인용하면 안 됩니다. 아르헨티나 페소·브라질 헤알·콜롬비아 페소로 거래소에서 산 자산의 절반 이상이 스테이블코인이었습니다 ([Chainalysis LATAM, 2025-10-02](https://www.chainalysis.com/blog/latin-america-crypto-adoption-2025/)). 베네수엘라에서는 USDT가 "바이낸스 달러"로 불리며 식료품·관리비·급여에 쓰인다는 보도가 있으나 2차 출처입니다 **[미검증]** ([Cointelegraph, 2025](https://cointelegraph.com/news/usdt-binance-dollars-replace-bolivar-in-venezuela)). BIS는 2024년 스테이블코인 국경 간 흐름을 약 1.4조 달러로 추정하고, 자본 통제의 영향을 받지 않는 흐름이라는 점에서 **규제 단속 위험**을 함께 지적했습니다 **[미검증: 검색 요약]** ([BIS WP 1370](https://www.bis.org/publ/work1370.htm)). 송금 비용은 200달러 기준 전 세계 평균 약 6.4~6.5%로 SDG 목표 3%의 두 배이지만, 세계은행 데이터에서 가장 싼 수단은 모바일 머니이고, "스테이블코인 송금이 약 60% 싸다"는 주장은 거래소 임원의 발언이지 측정치가 아닙니다 ([Chainalysis SSA 2024](https://www.chainalysis.com/blog/subsaharan-africa-crypto-adoption-2024/); [Remitbee 요약(2차)](https://www.remitbee.com/blog/money-transfer/remittance/top-remittance-corridors-2025-fees-market-share)).

정리하면, 이 수요가 말해 주는 것은 **"자국 통화가 불안정한 사람들이 달러 스테이블코인을 저축·송금 수단으로 들고 있다"**입니다. 이 사람들에게 필요한 것은 결제망이 아니라 **들고 있는 달러 잔액을 안전하게 지키고 필요할 때 정확하게 보내는 도구**이며, 이 지점에서 하드웨어 월렛의 역할이 생깁니다.

---

## 2. 카드사와 PG사는 스테이블코인을 뒷단에 흡수했고, 사용자 키는 쥐여 주지 않았습니다

2024~2026년 카드사·PG사의 움직임은 한 방향입니다. **가맹점은 계속 법정화폐를 받고, 스테이블코인은 카드 소지자의 잔액 쪽과 기관 간 정산 쪽에만 들어갑니다.**

| 기업 | 주요 움직임(날짜) | 사용자 키 구조 | 출처 |
|---|---|---|---|
| Visa | USDC 정산 연환산 35억 달러(2025-11-30) → 70억 달러, 9개 체인(2026-04-29) → **200억 달러 초과, 전년 대비 15배 이상, 스테이블코인 연동 카드 160개 이상(2026-09-08)** | 카드 프로그램 운영사(Bridge, Rain, Baanx)가 승인 시 전환, 가맹점은 법정화폐 수령 | [Visa, 2025-12-16](https://usa.visa.com/about-visa/newsroom/press-releases.releaseId.21951.html); [Visa IR, 2026-04-29](https://investor.visa.com/news/news-details/2026/Visa-Accelerates-Stablecoin-Momentum-Adding-Five-Blockchains-for-Settlement/default.aspx); [The Block, 2026-09-08](https://www.theblock.co/news/business/2026-09-08-visa-stablecoin-settlement-tops-20-billion-annualized-run-rate-up-more-than-15x-year-over-year-413749) |
| Mastercard | 엔드투엔드 스테이블코인 기능, MetaMask·OKX 등 지갑 카드(2025-04-28), Crypto Partner Program 85개사 이상(2026-03), BVNK 최대 18억 달러 인수 합의(2026-03-17), SoFiUSD 온체인 정산 실가동(2026-09-22) | 지갑 연동 카드는 온체인 승인(allowance) 기반, 정산 키는 기관 보관 | [Mastercard, 2025-04-28](https://www.mastercard.com/news/press/2025/april/mastercard-unveils-end-to-end-capabilities-to-power-stablecoin-transactions-from-wallets-to-checkouts/); [CNBC, 2026-03-17](https://www.cnbc.com/2026/03/17/mastercard-acquiring-stablecoin-startup-bvnk-in-crypto-bet.html); [PYMNTS, 2026-09](https://www.pymnts.com/cryptocurrency/2026/mastercard-sofi-bring-stablecoin-settlement-cards-step-one-broader-rollout/) |
| Stripe | Bridge 11억 달러 인수 완료(2025-02-04), Stablecoin Financial Accounts 101개국(2025-05), Privy 인수(2025-06), 자체 L1 Tempo 메인넷(2026-03-18) | 수탁형 또는 시드 없는 내장 지갑(MPC 계열) | [Stripe, 2025-02-04](https://stripe.com/newsroom/news/stripe-completes-bridge-acquisition); [Stripe Sessions 2025](https://stripe.com/newsroom/news/sessions-2025); [CoinDesk, 2026-03-18](https://www.coindesk.com/tech/2026/03/18/stripe-led-payments-blockchain-tempo-goes-live-with-protocol-for-ai-agents) |
| PayPal | Pay with Crypto: 미국 가맹점이 100종 이상 코인 수용, 수수료 0.99%, 법정화폐 또는 PYUSD 정산(2025-07-28) | 외부 지갑에서 송금, 전환은 PayPal이 수행 | [PayPal, 2025-07-28](https://newsroom.paypal-corp.com/2025-07-28-PayPal-Drives-Crypto-Payments-into-the-Mainstream,-Reducing-Costs-and-Expanding-Global-Commerce) |
| Shopify·Coinbase | Base 체인 USDC 결제, 34개국 조기 도입(2025-06-12) | 구매자 자기 지갑 가능, 가맹점 기본값은 법정화폐 | [Coinbase, 2025-06-12](https://www.coinbase.com/blog/coinbase-and-shopify-bring-usdc-payments-on-base-to-millions-of-merchants-worldwide) |
| 한국 | 카카오 원화 스테이블코인 지갑 계획(2026-05), 카카오페이·카카오뱅크–Fireblocks MoU(2026-09-21), 토스–Optimism PoC(2026-07), KB–KG이니시스–Kaia PoC | 플랫폼 앱 내장 지갑, 원화 스테이블코인 법안 미통과 | [Seoul Economic Daily, 2026-05-28](https://en.sedaily.com/finance/2026/05/28/kakao-to-launch-won-stablecoin-wallet-on-kakaobank-app); [UPI, 2026-09-08](https://www.upi.com/Top_News/World-News/2026/09/08/banks-crypto-exchanges-payment-platforms-won-stablecoin/2011788912171/); [Coinpaprika(2차)](https://coinpaprika.com/news/kakao-pay-kakaobank-sign-stablecoin/) |

해석할 때 주의할 점이 세 가지 있습니다. 첫째, Visa의 200억 달러는 최근 활동을 연환산한 수치이지 한 해 실적이 아니며, Visa 전체 결제액(조 달러 단위)에 비하면 작습니다. 의미는 규모보다 증가 속도, 즉 카드 발급사가 카드 자금(float)을 스테이블코인으로 옮기고 있다는 데 있습니다 ([KuCoin 블로그(2차)](https://www.kucoin.com/blog/visa-stablecoin-adoption-20-billion-settlements)). 둘째, Mastercard의 BVNK 인수는 Fortune(2026-03-17)이 "연내 종결 예정"이라고 보도했지만 일부 2차 출처는 "완료"로 표기해 **[상충]**이며, 종결 여부를 확인하지 못했습니다 ([Fortune, 2026-03-17](https://fortune.com/2026/03/17/mastercard-bvnk-acquisition-stablecoins-1-8-billion/)). Checkout.com의 스테이블코인 수용(2026-06)은 집계 사이트에서만 확인되어 **[미검증]**입니다. 셋째, 미국 GENIUS Act(2025-07-18 서명), 홍콩 스테이블코인 조례(2025-08-01 시행), 일본 JPYC(2025-10-27 출시)처럼 발행사 인가 제도가 먼저 생긴 곳에서 카드사가 "규제된 스테이블코인" 정산을 시작했습니다 ([Covington](https://www.cov.com/en/news-and-insights/insights/2025/07/the-genius-act-becomes-law-key-provisions-from-the-federal-stablecoin-regulatory-framework); [HK Gov](https://www.info.gov.hk/gia/general/202506/06/P2025060600275.htm)). 한국 원화 스테이블코인은 법안이 아직 통과되지 않았습니다.

이 표에서 프로젝트에 중요한 사실은 하나입니다. **조사한 어떤 카드사·PG 제품도 하드웨어 월렛을 결제 수단으로 쓰지 않습니다.** 자기 보관(self-custody)에 가장 가까운 MetaMask 카드조차 사용자가 한도를 온체인으로 승인(approve)해 두면 운영사가 카드 승인 시점에 자금을 끌어가는 구조입니다 ([CoinDesk, 2025-04-30](https://www.coindesk.com/business/2025/04/30/visa-and-baanx-launch-usdc-stablecoin-payment-cards)). 카드망 승인은 1~2초 안에 끝나야 하고, 환불·차지백과 법정화폐 정산이 필요하기 때문에 업계는 건별 서명 대신 사전 위임 모델로 수렴했습니다. 이것이 "시장 공백"인지 "시장이 이유가 있어서 피한 설계"인지는 구분해야 합니다. 건별 하드웨어 서명의 어려운 점은 키 보안이 아니라 **판매자 인증, 결제 확정(finality) 대기 시간, 가스비 처리**입니다.

---

## 3. 앱 기반 QR 결제는 키 보관, 주소 확인, 서명 해석 세 곳에서 뚫립니다

### 피해 규모

Chainalysis는 2025년 개인 지갑 탈취를 **15.8만 건(피해자 최소 8만 명), 7.13억 달러**로 집계했습니다. 건수는 2022년 5.4만 건에서 세 배로 늘었고, 금액은 2024년 15억 달러에서 줄었습니다. Chainalysis의 표현대로 "더 많은 사용자를 노리고 1인당 더 적게 훔치는" 양상입니다 ([Chainalysis, 2025-12-18](https://www.chainalysis.com/blog/crypto-hacking-stolen-funds-2026/)). 소액·다빈도 결제 지갑은 정확히 이 대량·저액 공격의 표적입니다. 서명 피싱(drainer) 피해는 2025년 8,385만 달러, 피해자 10.6만 명으로 2024년 대비 83% 줄었지만, 100만 달러 이상 사건의 38%가 Permit/Permit2 서명에서 나왔고 최대 단일 피해는 Permit 서명 한 번으로 650만 달러였습니다. 2025년 8월에는 EIP-7702 위임 피싱으로 254만 달러가 털렸습니다 ([Scam Sniffer 2025 보고서](https://drops.scamsniffer.io/scam-sniffer-2025-crypto-phishing-losses-fall-83-to-84-million/)).

### 휴대폰은 키 저장소로 안전하지 않습니다

모바일 경로는 실제 사례로 확인됩니다. SparkCat과 SparkKitty는 **애플 App Store와 구글 Play 공식 스토어**를 통과해 사진첩을 훑고, OCR로 글자가 있는 이미지만 골라 유출했습니다. 시드 문구를 사진으로 찍어 두는 습관이 곧 키 유출이 됩니다 ([Kaspersky Securelist, 2025-06-23](https://securelist.com/sparkkitty-ios-android-malware/116793/)). Ledger Donjon이 공개한 CVE-2025-20435는 Trustonic TEE를 쓰는 MediaTek 기반 안드로이드(추정 약 25%)에서 부팅 전 USB로 접근해 **약 45초 만에 핫월렛 앱의 시드를 추출**할 수 있고, 전원이 꺼진 폰에도 통합니다. 패치는 모든 기기에 배포되지 않았습니다 ([The Block, 2026-03-11](https://www.theblock.co/post/393154/ledger-researchers-expose-android-flaw-enabling-theft)). Ledger는 이해당사자이므로 "스마트폰은 키 저장에 부적합하다"는 Ledger CTO의 표현은 그 점을 감안해 인용해야 합니다. 구글 Play 가짜 지갑 앱 20종(Cyble, 2025)은 2차 출처뿐이라 **[미검증]**입니다. 저가폰·공유폰은 패치되지 않은 OS, 사이드로딩, 여러 사용자의 접근이 겹쳐 가장 취약한 환경입니다.

### 주소 바꿔치기와 QR 스티커

USENIX Security 2025 논문은 이더리움·BSC에서 2년간 **2.7억 건 이상의 주소 오염(address poisoning) 시도, 피해 대상 1,700만 명 이상, 성공 이체 6,633건, 8,380만 달러 이상**을 측정했습니다 ([USENIX, Tsuchiya et al.](https://www.usenix.org/system/files/usenixsecurity25-tsuchiya.pdf)). 주차 요금기에 가짜 QR 스티커를 덧붙이는 사기는 2025~2026년 뉴욕·레돈도비치·애슈빌 등에서 확인되었지만 **이 사례들은 카드 피싱 페이지로 유도한 것이고, 크립토 주소 QR을 바꿔치기해 손실이 난 1차 출처 사례는 찾지 못했습니다** ([NY1, 2025-06-09](https://ny1.com/nyc/all-boroughs/news/2025/06/09/nyc-parking-meter-scam-fake-qr-codes)). 다만 EIP-681 결제 URI에는 판매자 인증 필드가 없으므로 ([EIP-681](https://eips.ethereum.org/EIPS/eip-681)), 고정 QR은 주소 하나만 바꾸면 되고 손실은 되돌릴 수 없습니다. 이 위험은 구조적으로는 명확하지만 피해 통계로는 아직 입증되지 않았다는 점을 구분해 두십시오.

### 결제 기능 자체의 공백

QR 크립토 결제에는 카드 결제가 당연히 가진 기능이 빠져 있습니다. 같은 주소 형식이 여러 EVM 체인에 존재하고 USDC·USDT가 여러 체인에 배포되어 있어 체인 ID와 토큰 컨트랙트를 무시하면 다른 체인으로 송금됩니다. 스테이블코인만 가진 사용자는 네이티브 가스 토큰이 없으면 결제할 수 없습니다. 판매자는 멤풀·첫 블록 포함 시점에 받을지(재정렬 위험) 확정까지 기다릴지(계산대 지연) 골라야 합니다. 차지백이 없어 환불은 새 송금이 되고, 주문별 참조값이 없으면 동시 주문을 금액과 시각으로 맞춰야 합니다. Solana Pay는 세션별 `reference`로 조정(reconciliation) 문제를 해결했지만 ([Solana Pay spec](https://docs.solanapay.com/spec)), EVM 쪽에는 서명된 판매자 결제 요청 표준이 널리 쓰이지 않습니다. 이 항목들은 프로토콜 성질에서 도출한 추론이고, 오송금률이나 계산대 이탈률 같은 정량 데이터는 찾지 못했습니다.

---

## 4. 하드웨어 월렛은 키 추출을 막지만, 사람이 무엇에 서명하는지는 보장하지 않습니다

### 한계부터

Bybit 사건(2025-02-21, **약 14억~15억 달러 [상충: 출처별 14억 달러 이상 / 약 15억 달러]**)이 핵심 반례입니다. 북한 Lazarus가 Safe{Wallet} 개발자 PC를 장악해 AWS S3의 프런트엔드 JS를 바꿨고, 이 스크립트는 Bybit 콜드월렛 Safe에서만 작동하며 **화면에는 원래 값을 보여 주면서 실제 트랜잭션을 악성 컨트랙트 업그레이드로 바꿨습니다.** 서명자들은 Ledger 기기를 썼고 키는 전혀 유출되지 않았지만, 기기가 실제 내용을 해석해 보여 주지 않아 블라인드 서명을 했습니다 ([Sygnia](https://www.sygnia.co/blog/sygnia-investigation-bybit-hack/); [Ledger, 2025-03-27](https://www.ledger.com/blog-learning-from-the-bybit-safe-attack); [NCC Group](https://www.nccgroup.com/research/in-depth-technical-analysis-of-the-bybit-hack/)). 하드웨어 서명기는 **기기 화면이 서명할 바이트를 사람이 읽을 수 있는 의도로 해석해 보여 줄 때만** 호스트 조작을 막습니다.

하드웨어 월렛이 막지 못하는 것은 다음과 같습니다. 주소 오염·QR 바꿔치기·클리퍼는 기기가 공격자 주소를 "정직하게" 보여 줄 뿐이어서, 기기 내 주소록이나 판매자 신원 검증이 없으면 막히지 않습니다. 시드를 직접 입력하게 만드는 사회공학(가짜 "양자 내성 업데이트" 우편물 등)도 막지 못합니다 ([HackRead](https://hackread.com/scammers-physical-phishing-letters-ledger-wallet-seed/)). 결제 레일의 스마트 컨트랙트 버그도 키 기기와 무관합니다. Gnosis Pay는 2026-06-01 zodiac-core의 `staticcall` 성공 여부 미확인 버그(2023-10-28 커밋에서 유입, 2026-02-27 조용히 수정)로 공격받았고, 피해액은 Gnosis 발표 약 150만 달러(+약 30만 달러 접근 불가)와 CertiK 인용 보도 약 26.5만 달러로 **[상충]**합니다 ([Gnosis Pay post-mortem](https://gnosispay.com/blog/post-mortem-gnosis-pay-vulnerability-exploit); [CryptoTimes, 2026-06-05](https://www.cryptotimes.io/2026/06/05/delay-module-trick-costs-gnosispay-265k-reports-certik/)). 그리고 소액 결제에서 기기를 꺼내고 PIN을 누르는 마찰과 기기 비용은 실제 단점입니다. 하드웨어 월렛 사용자와 핫월렛 사용자의 손실률을 비교한 독립 통계는 찾지 못했습니다.

### 장점

반대로 하드웨어 월렛은 앞 장의 모바일 공격 경로를 구조적으로 닫습니다. 키가 휴대폰에 없으므로 악성 앱·인포스틸러·CVE-2025-20435 같은 물리 추출이 무력화되고, 기기가 시드를 휴대폰 화면에 내보내지 않으면 OCR 탈취 경로도 사라집니다. 공유폰 환경에서도 PIN과 물리 버튼 확인이 기기에 있으므로 다른 사용자가 폰을 만져도 결제할 수 없습니다. ERC-7730은 컨트랙트 함수를 사람이 읽을 수 있는 필드로 매핑하는 JSON 기술자(descriptor)를 공개 레지스트리에 두고 기기 화면에서 해석해 보여 주는 표준으로, Ledger가 기여했고 거버넌스는 이더리움 재단으로 이전 중입니다 ([Ledger, ERC-7730 v2](https://www.ledger.com/blog-the-evolution-of-clear-signing); [레지스트리](https://github.com/ethereum/clear-signing-erc7730-registry)). 다만 신뢰가 호스트 UI에서 레지스트리로 옮겨 가는 것이므로, 레지스트리가 오염되거나 기술자가 없으면 다시 블라인드 서명이 됩니다.

### 결제용 기존 형태

결제를 표방하는 기존 제품은 모두 "사용자 키는 한도를 정하고, 발급사의 제한된 키가 카드 결제를 실행한다"는 구조입니다. Tangem Pay는 칩 키와 발급 파트너 키를 쓰는 2키 구조로 Polygon USDC를 Visa 카드로 소비하고, Gnosis Pay는 Safe의 Roles 모듈 일일 한도와 약 3분 Delay 모듈을 쓰며, MetaMask 카드는 온체인 승인, ether.fi는 Safe 금고 한도와 보안 지연을 씁니다 ([Coin Bureau, Tangem Pay](https://coinbureau.com/education/what-is-tangem-pay); [Gnosis Pay help](https://help.gnosispay.com/hc/en-us/articles/39400440331668-How-the-Delay-and-Roles-Modules-Secure-Your-Card)). Ledger CL 카드도 Baanx 발급 Visa 직불카드로 기기에서 카드 계정으로 자금을 옮기는 방식입니다 ([Ledger CL Card](https://shop.ledger.com/pages/cl-card-crypto-card)). Tangem Pay가 결제마다 카드 탭을 요구하는지는 1차 출처로 확인하지 못했습니다.

---

## 5. 기존 하드웨어 월렛의 사고는 칩 바깥에서 났습니다

### 제품별 보안 구조

| 제품 | 보안 구조 | 연결 | 가격(시점 혼재) | 출처 |
|---|---|---|---|---|
| Ledger Nano Gen5 / Flex / Stax | ST33K1M5 SE(CC EAL6+)에서 서명, 화면도 SE가 구동, OS(BOLOS) 비공개 | USB-C, BT, NFC | $179 / $249 / $399 | [Ledger Academy](https://www.ledger.com/academy/the-world-of-nano-which-ones-for-you); [Coin Bureau](https://coinbureau.com/analysis/ledger-stax-vs-ledger-flex) |
| Trezor Safe 7 | STM32U5 MCU + 이중 SE(개방 설계 TROPIC01 + Infineon Optiga EAL6+), 펌웨어 오픈소스·재현 빌드 | USB-C, BT | $249 | [Trezor Safe 7](https://trezor.io/trezor-safe-7) |
| Trezor Safe 3 / 5 | MCU + Optiga Trust M(EAL6+) | USB-C | $59~79 / $129 **[상충: 지역별 가격]** | [CryptoSlate](https://cryptoslate.com/crypto-wallets/trezor-safe-3-vs-safe-5/) |
| Keystone 3 Pro | 제조사 설명: ATECC608B + DS28S60 + MAX32520 3개 SE (리뷰는 "Infineon 3개"로 **[상충]**, 제조사 설명 우선), QR 전용 에어갭 | QR만 | 약 $129~149 | [Keystone blog](https://blog.keyst.one/inside-the-vault-how-keystone-3-pro-secures-your-crypto-with-triple-se-chips) |
| Tangem 카드·링 | Samsung S3D350A SE(EAL6+), 화면 없음(폰이 UI), 시드 없는 모드 | NFC만 | 카드 2장 $59.90 | [Tangem pricing](https://tangem.com/en/pricing/) |
| Coldcard Mk4/Q/Mk5 | MCU + 이중 SE(ATECC608A + DS28C36B), 비트코인 전용, 펌웨어 공개 | USB-C, NFC, microSD | Mk5 $167 | [Coldcard Mk4](https://coldcard.com/mk4); [Coinkite, 2026-03-10](https://blog.coinkite.com/coldcard-mk5-launch/) |
| BitBox02 Nova | ATSAMD51 MCU + Optiga Trust M V3(EAL6+), 펌웨어·회로도 공개, anti-klepto 구현 | USB-C, BLE | €175 | [BitBox Nova](https://blog.bitbox.swiss/en/introducing-bitbox02-nova/) |

시장은 SE 안에서 서명까지 하는 폐쇄형(Ledger, Tangem)과, 범용 MCU가 서명하고 SE는 비밀을 지키는 개방형(Trezor, BitBox, Coldcard, Keystone)으로 나뉩니다. 이 프로젝트가 nRF54L15로 만들 수 있는 것은 후자뿐입니다. NFC 탭과 기기 자체 보안 화면을 함께 가진 제품(Ledger Gen5/Flex, Bitkey 2026)이 "탭해서 서명하는 결제 기기"에 가장 가까운 형태이고, Tangem은 탭 UX는 입증했지만 화면이 폰에 있어 "보이는 것에 서명한다(WYSIWYS)"는 원칙이 약합니다.

### 알려진 사고와 그 원인

사고를 원인별로 모으면 칩 파괴는 소수입니다.

**펌웨어 결함이 가장 큰 금액을 냈습니다.** Coldcard는 2021년 3월 빌드에서 `MICROPY_HW_ENABLE_RNG`가 0으로 설정되어 시드 생성이 칩 UID와 타이머로 시드된 MicroPython Yasmarang PRNG로 떨어졌고, Mk4/Q/Mk5 시드가 128비트가 아닌 약 72비트 강도가 되었습니다. 2026-07-30 원격 무차별 대입으로 첫 41분에 1,082.65 BTC(약 7,020만 달러)가 빠져나갔고, 피해 집계는 1,367.05 BTC·약 8,860만 달러·4,585개 주소(The Hacker News), 약 1억 달러(CoinDesk), 1,778.6 BTC·1.12억 달러 초과·8,600개 이상 주소(Galaxy Research 인용)로 **[상충]**합니다. 패치는 이미 만든 시드를 고치지 못하므로 사용자가 새 시드로 자금을 옮겨야 합니다 ([The Hacker News, 2026-08](https://thehackernews.com/2026/08/coldcard-hardware-wallet-flaw-linked-to.html); [CoinDesk, 2026-08-17](https://www.coindesk.com/tech/2026/08/17/how-a-bug-in-coldcard-s-code-went-unnoticed-for-years-leading-to-usd100-million-in-hacked-funds); [FinanceFeeds](https://financefeeds.com/bitbox-wallet-flaws-emerge-after-coldcard-exploit-led-to-112m-in-losses/)). 오픈소스와 재현 빌드는 바이너리의 출처를 증명할 뿐 정확성을 증명하지 않는다는 반례입니다. Dark Skippy(2024-08-05)는 악성 펌웨어가 서명 nonce에 시드를 심어 **서명 두 번으로 12단어 시드를 유출**할 수 있음을 보였고, 대응책은 호스트가 제공한 난수에 nonce를 묶는 anti-exfil(anti-klepto)입니다 ([darkskippy.com](https://darkskippy.com/); [BitBox anti-klepto](https://bitbox.swiss/blog/anti-klepto-explained-protection-against-leaking-private-keys/)). BitBox02는 2026년 설정 전 메모리 손상과 Silent Payments 주소 버그를 패치했고 악용 증거는 없었습니다(CVE 미확인).

**호스트 소프트웨어 공급망**이 두 번째입니다. Ledger Connect Kit(2023-12-14)는 전 직원이 피싱당해 npm 세션 토큰을 빼앗기고 악성 1.1.5~1.1.7 버전이 약 5시간 배포되어 약 48.4만 달러(이후 보도 60만 달러 이상, **[상충]**)가 털렸습니다 ([Ledger 사고 보고](https://www.ledger.com/blog/security-incident-report); [CoinDesk, 2023-12-14](https://www.coindesk.com/business/2023/12/14/ledger-exploit-drained-484k-upended-defi-former-staffer-linked-to-malicious-code)). Bybit도 같은 계열입니다.

**고객 개인정보 유출**이 피싱과 물리 공격의 표적 목록이 되었습니다. Ledger는 2020년 약 110만 이메일과 27.2만 건의 이름·전화·주소가 유출되었고, 이후 가짜 지원 문자, 협박, 플래시 드라이브를 납땜한 가짜 Nano 우편 발송(2021), 시드 입력을 요구하는 가짜 "보안 업데이트" 우편물(2025-04 이후)로 이어졌습니다 ([Ledger](https://www.ledger.com/addressing-the-july-2020-e-commerce-and-marketing-data-breach); [CoinDesk, 2021-06-17](https://www.coindesk.com/tech/2021/06/17/scammers-are-sending-ledger-users-fake-hardware-wallets); [The Block, 2025-04-30](https://www.theblock.co/news/ecosystems/2025-04-30-ledger-confirms-physical-scam-letters-requesting-seed-phrase-352479)). Trezor는 2025년 6월 지원 티켓 자동응답이 공격자 제목을 실어 공식 주소에서 발송되는 피싱에 악용되었고, 2026년 배송사 ShipMonk를 통해 13,689명, SafePal은 주문 추적 플러그인으로 39,798명의 정보가 노출되었습니다 ([BleepingComputer](https://www.bleepingcomputer.com/news/security/trezors-support-platform-abused-in-crypto-theft-phishing-attacks/); [FinanceFeeds](https://financefeeds.com/bitbox-wallet-flaws-emerge-after-coldcard-exploit-led-to-112m-in-losses/)). 물리적 강탈("렌치 공격")은 2025년 기록적으로 늘었고 Ledger 공동창업자가 2025년 1월 납치되었지만, 2025년 건수는 출처별로 약 25건(첫 21주)과 약 70건으로 **[상충]**합니다 ([jlopp/physical-bitcoin-attacks](https://github.com/jlopp/physical-bitcoin-attacks); [DL News](https://www.dlnews.com/articles/regulation/ledger-cofounder-david-balland-and-wife-kidnapped-in-france/)).

**신뢰 정책 위반**도 사고입니다. Ledger Recover(2023-05-16 발표, 5-23 연기)는 암호화된 시드 조각을 세 수탁사에 맡기는 선택형 서비스였지만, "키는 SE 밖으로 나가지 않는다"는 약속과 달리 펌웨어에 시드 내보내기 경로가 있다는 사실이 드러나 신뢰를 잃었습니다 ([CoinDesk, 2023-05-24](https://www.coindesk.com/tech/2023/05/24/ledger-recover-fiasco-exposes-gap-between-blockchain-ideals-and-technical-reality)).

**칩 수준 물리 공격**은 SE가 없는 MCU에서 반복되었습니다. Trezor One/T는 STM32 플래시에 시드를 두었고 wallet.fail(2018), Kraken(2020, 물리 접근 약 15분), Ledger Donjon(2020), Unciphered(2023)가 전압 글리칭으로 뚫었습니다. 이 결함은 실리콘에 있어 소프트웨어로 고칠 수 없고, 완화책은 기기에 없는 비밀(패스프레이즈)뿐이었습니다 ([Kraken, 2020-01-31](https://blog.kraken.com/product/security/kraken-identifies-critical-flaw-in-trezor-hardware-wallets); [Ledger Donjon](https://www.ledger.com/blog/unfixable-key-extraction-attack-on-trezor)). SE가 있는 Trezor Safe 3에서도 MCU 글리칭으로 펌웨어 진위 검사는 우회되었지만 **PIN과 키는 SE가 지켰습니다** ([Trezor advisory](https://trezor.io/vulnerability/donjon-s-trezor-safe-3-evaluation)). 이것이 외부 SE를 붙여야 하는 가장 직접적인 근거입니다.

---

## 6. nRF54L15 단독으로는 시드를 지킬 수 없습니다

nRF54L15는 MCU로서 좋은 보안 기능을 갖췄습니다. Cortex-M33 TrustZone, 사이드채널 대응을 표방하는 암호 가속기·TRNG인 CRACEN, KMU 키 슬롯, 보안 부팅, 하드웨어·소프트웨어 AP-Protect, 글리치 검출기와 액티브 실드(4쌍)를 가진 TAMPC가 있습니다 ([Nordic 제품 페이지](https://www.nordicsemi.com/Products/nRF54L15); [NCS ap_protect](https://docs.nordicsemi.com/bundle/ncs-3.2.0/page/nrf/security/ap_protect.html)). 그러나 네 가지 한계가 있습니다.

첫째, Nordic은 "PSA Certified Level 3를 목표로 설계"라고 홍보하지만 **실제 PSA Certified 데이터베이스에는 Level 1(2026-01-15, SERMA, NCS v3.1/TF-M v2.1.2/MCUboot v2.2.0 기준)만 등록**되어 있습니다 ([PSA Certified DB](https://products.psacertified.org/products/nrf54l15-nrf54l10)). 물리 공격 저항성을 독립 평가받은 CC EAL5+/6+ 보안 칩이 아닙니다. 둘째, CRACEN은 P-256 ECDSA/ECDH를 지원하지만 **EVM과 비트코인이 쓰는 secp256k1 하드웨어 지원은 확인되지 않았습니다** ([NCS crypto drivers](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/security/crypto/drivers.html)). 그러면 서명은 M33에서 소프트웨어로 돌고, 개인키는 KMU가 아니라 일반 RAM·RRAM에 놓입니다. 이 저장소의 기존 문서(`board-fit-research.md`)도 NU 보드에서 secp256k1 비추출 키 서명 경로를 확인하지 못했다고 적고 있어 같은 결론입니다. NCS 2.9.1에서 KMU로의 `psa_generate_key`가 `PSA_ERROR_NOT_SUPPORTED`를 반환했다는 보고도 하나 있습니다 ([DevZone](https://devzone.nordicsemi.com/f/nordic-q-a/124560/psa_generate_key-into-kmu-fails-on-nrf54l15-ncs-2-9-1-with-psa_error_not_supported)). 셋째, 이전 세대 nRF52는 전압 글리칭으로 APPROTECT가 우회되었고(CVE-2020-27211) 이 공격은 AirTag 펌웨어 덤프와 ESP32 기반 해제 도구로 상품화되었습니다 ([LimitedResults](https://www.limitedresults.com/results/nrf52-debug-resurrection-approtect-bypass)). nRF54L에 대한 공개 글리칭 연구는 2026-09까지 찾지 못했는데, 이것은 "안전하다"가 아니라 "아직 평가되지 않았다"는 뜻입니다. 넷째, Nordic 문서 사이트가 403을 반환해 KMU 슬롯 수와 CRACEN 알고리즘 목록은 검색 요약으로만 확인했으므로 팀이 데이터시트 v1.0을 직접 읽어야 합니다 ([데이터시트 미러](https://www.mouser.lt/datasheet/3/926/1/nRF54L15_nRF54L10_nRF54L05_Datasheet_v1.0.pdf)).

따라서 현실적인 구조는 Trezor Safe 방식입니다. **nRF54L15는 BLE·화면·버튼·서명 로직을 맡고, 외부 SE(Infineon OPTIGA Trust M, NXP SE050, TROPIC01 중 택1)는 PIN으로만 열리는 래핑 비밀과 하드웨어 재시도 카운터를 가지며, 시드는 RRAM에 암호화된 상태로만 존재**합니다. 플래시를 통째로 덤프해도 SE의 재시도 제한 때문에 PIN을 무차별 대입할 수 없습니다. 이 구조의 단점도 분명합니다. 잠금이 풀린 세션 동안 MCU가 장악되면 키는 노출되고, SE와 MCU 사이 I2C/SPI 버스는 페어링·암호화하지 않으면 도청됩니다. SE050의 EAL6+ 인증, ATECC608의 CC 미인증, 각 SE의 secp256k1 지원 여부는 1차 인증서로 확인하지 못했습니다 **[미검증]**. TROPIC01은 "EAL5+ 상당"으로 표기될 뿐 정식 CC 인증서가 아닙니다 ([Tropic Square](https://www.tropicsquare.com/tropic01)). NU-54V-DK 보드에 SE가 실장되어 있는지는 이번 자료에 없으므로 확인이 필요하며, 없다면 외부 SE 브레이크아웃 보드를 붙여야 합니다.

---

## 7. 프로젝트 정의

### 먼저 버려야 할 발표 문장

"금융 인프라가 부족한 국가에서 하드웨어 월렛으로 매장 결제를 하면 유용하다"는 문장은 쓰지 않는 것을 권장합니다. 엘살바도르 사례, 카드 소비 45억 달러, 모바일 머니 5.93억 활성 계정, 3.7억 피처폰 사용자라는 네 가지 반박이 준비되어 있기 때문입니다. 대신 근거가 뒷받침하는 문장은 "불안정한 통화 국가에서 달러 스테이블코인을 저축·송금 수단으로 들고 있는 사람이 늘고 있고, 이들이 쓰는 앱 지갑은 휴대폰 악성코드·주소 바꿔치기·블라인드 서명에 취약하다"입니다. 12주 PoC가 보여줄 수 있는 것은 수요가 아니라 **보안 설계**입니다.

### 목표 문제 정의 후보

| 후보 | 문제 정의 | 장점 | 단점(trade-off) |
|---|---|---|---|
| A. 판매자 인증 결제 서명기 | "판매자 단말이 만든 주문별 결제 요청을 기기가 받아, 판매자 신원·토큰·금액·체인을 기기 화면에서 해석해 보여 주고 물리 버튼으로 확인한 경우에만 서명한다. 고객은 스마트폰 없이 기기만 들고 오고, 네트워크 연결과 브로드캐스트는 판매자 단말이 맡는다." | 기존 카페 키오스크 자산(판매자 단말, 테스트넷 연동)을 재사용합니다. 고객 스마트폰이 필요 없으므로 피처폰 사용자 문제에 대한 설계 답을 제시할 수 있습니다(가설). QR 바꿔치기·블라인드 서명·폰 악성코드를 한 시연에서 모두 다룹니다. | 매장 결제 수요가 작다는 근거와 정면으로 부딪힙니다. 결제 확정 대기, 가스비, 환불이라는 시장이 피한 난제를 떠안습니다. 판매자 단말을 신뢰해야 하는 새 공격면이 생깁니다. |
| B. 스테이블코인 금고 + 한도 위임 | "사용자의 달러 스테이블코인 저축을 기기가 지키고, 기기는 송금·한도 설정·한도 변경만 서명한다. 일상 소액 결제는 기기가 설정한 한도 안의 세션 키나 스마트 계정 모듈이 처리한다." | 개도국 수요(저축·송금)와 업계 수렴 구조(Tangem Pay, Gnosis Pay, MetaMask 카드)에 맞습니다. 결제 확정 대기와 매 결제마다 기기를 꺼내는 마찰이 사라집니다. | 매장 결제 시연이 약해집니다. 스마트 계정·모듈 컨트랙트 개발이 추가되고, Gnosis Pay 사례처럼 컨트랙트 버그가 기기 보안과 무관한 새 위험이 됩니다. 12주 3인에게 컨트랙트 감사 여력이 없습니다. |

A를 권장합니다 [Mid]. 이유는 두 가지입니다. 첫째, 이미 판매자 단말과 테스트넷 결제 흐름이 있어 12주 안에 끝까지 동작하는 시연을 만들 가능성이 높습니다. 둘째, 이 프로젝트의 차별점은 "하드웨어 키가 건별 결제를 직접 서명하고, 그 요청의 판매자를 기기가 검증한다"인데 이것은 조사한 어떤 제품에도 없습니다. 단, 발표에서 "수요 입증"이 아니라 "보안 설계 시연"으로 위치를 명시해야 하며, B의 한도 개념(요구사항 11)은 A 안에 기기 측 한도로 흡수합니다. 체인은 저장소 기존 문서가 정한 StableNet 테스트넷(체인 ID 8283, EVM)을 전제로 합니다.

### 보안 요구사항

각 요구사항은 앞에서 확인한 사고·실패 유형에 대응합니다. PoC에서 반드시 구현할 것(필수)과 설계·문서로 대신할 수 있는 것(권장)을 구분했습니다.

1. **[필수] 시드 생성은 하드웨어 TRNG(CRACEN)에서만 하고, 릴리스 빌드에서 이를 검증합니다.** 빌드 설정에서 TRNG 비활성 시 컴파일이 실패하도록 단언하고, 실기기 출력에 통계 검정을 돌립니다. 대응: Coldcard RNG 결함(2021~2026, 8,860만~1.12억 달러).
2. **[필수] 기기 화면에 수신자, 토큰 컨트랙트와 심볼, 금액, 체인 ID, 판매자 이름을 해석해 보여 주고, 해석할 수 없는 데이터는 서명을 거부합니다.** 해시나 "데이터 있음"만 보여 주는 블라인드 서명 모드를 두지 않습니다. 대응: Bybit(약 14억~15억 달러), Ledger Connect Kit.
3. **[필수] 결제 키의 서명 범위를 ERC-20 `transfer`(또는 정해진 결제 구조체 하나)로 제한하고 `approve`, Permit/Permit2, EIP-7702 위임, 임의 컨트랙트 호출은 거부합니다.** 대응: Permit 계열이 100만 달러 이상 피싱 피해의 38%, EIP-7702 피싱 254만 달러(2025).
4. **[필수] 결제 요청은 판매자 키로 서명된 주문별 요청이어야 하고, 기기는 사전 등록된 판매자 공개키로 이를 검증합니다.** 요청에는 주문 참조값과 만료 시각을 넣어 재사용을 막습니다. 고정 QR·주소 문자열만으로는 서명하지 않습니다. 대응: 주소 오염(2.7억 건 시도, 8,380만 달러), QR 스티커 바꿔치기, 클리퍼.
5. **[필수] 서명 확인은 기기의 물리 버튼으로만 받습니다.** BLE로는 서명 전 요청과 서명 결과만 오가며, BLE 명령으로 확인을 대신할 수 없습니다. BLE는 LE Secure Connections(숫자 비교 또는 패스키)로 페어링합니다. 대응: 호스트 장악 일반, BLE 원격 공격면(구체 사례는 찾지 못함).
6. **[필수] 시드는 평문으로 비휘발 메모리에 저장하지 않습니다.** 시드는 외부 SE가 보관한 PIN 게이트 비밀로 암호화하고, PIN 재시도 제한은 SE가 강제합니다. 대응: Trezor One/T 전압 글리칭(Kraken 2020 등), Trezor Safe 3에서 SE가 PIN·키를 지킨 사례.
7. **[필수] 양산 설정과 같은 디버그 잠금을 PoC 보드에도 적용합니다.** 하드웨어·소프트웨어 AP-Protect, 보안 AP-Protect, TAMPC 글리치 검출을 켜고, 켜졌는지를 시연 체크리스트로 확인합니다. 대응: nRF52 APPROTECT 우회(CVE-2020-27211).
8. **[필수] 펌웨어는 MCUboot 서명 검증을 거쳐서만 부팅·업데이트하고, 펌웨어 어디에도 시드 내보내기 경로를 두지 않으며 그 정책을 문서로 공개합니다.** 대응: Ledger Recover 신뢰 훼손, 악성 펌웨어 업데이트.
9. **[필수] 시드는 기기 화면에만 표시하고 BLE나 앱으로 보내지 않습니다.** 앱 화면에 시드를 띄우는 기능을 만들지 않습니다. 대응: SparkCat·SparkKitty OCR 탈취, CVE-2025-20435(폰 저장소 45초 추출).
10. **[권장] 서명 nonce는 RFC 6979 결정적 nonce를 쓰고, 여유가 있으면 호스트 난수를 섞는 anti-exfil을 구현합니다.** RFC 6979만으로는 악성 펌웨어를 막지 못한다는 한계를 문서에 적습니다. 대응: Dark Skippy(서명 두 번으로 시드 유출).
11. **[필수] 기기 측 건당 한도와 일일 한도를 두고, 한도 변경은 PIN과 물리 확인을 다시 요구합니다.** 대응: 렌치 공격 시 피해 상한, 판매자 단말 장악 시 피해 상한.
12. **[권장] 호스트 앱·판매자 단말의 결제 표시가 단일 JS/npm 의존성에 의존하지 않도록 의존성을 고정(lock)하고, 최종 확인은 항상 기기 화면이라는 원칙을 UI에 명시합니다.** 대응: Ledger Connect Kit, Safe{Wallet} 프런트엔드 조작.
13. **[권장] 기기 진품 증명(attestation) 키를 두어 앱이 정품 기기와 서명된 펌웨어인지 확인합니다.** 대응: 가짜 Ledger 우편 발송(2021), Trezor Safe 3 공급망 검사 우회.
14. **[권장] PoC에서 사용자 이름·주소·전화 같은 개인정보를 수집하지 않습니다.** 대응: Ledger 2020 유출, Trezor/ShipMonk·SafePal 2026 유출이 피싱·물리 공격 표적 목록이 된 사례.

### 만들지 않는 것

- 메인넷 운영과 실제 자금 보관. 모든 시연은 테스트넷과 시연용 토큰으로 합니다.
- 법정화폐 입출금(on/off-ramp), 환전, KYC, 수탁 서비스. 규제 대상이고 12주 범위를 벗어납니다.
- 카드망(EMV) 연동과 카드 발급. BIN 스폰서와 발급 라이선스가 필요합니다.
- CC·PSA Level 3 인증 획득이나 "물리 공격에 안전하다"는 주장. nRF54L15는 PSA L1이고 외부 SE를 붙여도 평가받지 않은 조합입니다.
- 사이드채널·오류 주입 공격 실험실 수준 방어의 입증.
- 다중 체인·다중 자산 지원. 하나의 EVM 테스트넷과 하나의 ERC-20 토큰으로 제한합니다.
- USSD·SMS 기반 피처폰 연동의 실제 구현. 발표에서는 설계 방향으로만 언급합니다.
- 특정 국가 규제 준수 검토와 현지 시장 검증. 개도국 수요는 2차 자료로만 인용합니다.
- 시드 백업 서비스(Recover 류)와 클라우드 복구.
- 자체 스마트 계정·결제 모듈 컨트랙트(후보 B를 택하지 않는 경우).

---

## 결론

이번 조사로 바뀐 판단은 "어느 시장이 필요로 하는가"에서 "무엇이 실제로 뚫렸는가"로 질문을 옮겨야 한다는 것입니다. 개도국 결제 수요는 모바일 머니가 이미 차지했고 스테이블코인의 실제 용도는 저축과 송금이므로, 수요 논거로 이 프로젝트를 정당화하면 약합니다. 반면 2020~2026년 하드웨어 월렛 사고는 거의 모두 칩 바깥, 즉 무엇을 보여 주고 서명하는지, 난수를 제대로 쓰는지, 펌웨어가 무엇을 내보낼 수 있는지, 고객 정보가 어디에 남는지에서 났습니다. 3인 12주 팀이 기존 제품을 이길 수 있는 곳은 칩 보안이 아니라 이 영역이며, 판매자 인증 요청과 좁은 서명 범위는 대기업 제품도 결제 흐름에 적용하지 않은 부분입니다.

실행 측면에서 가장 먼저 확인할 것은 두 가지입니다. 첫째, 외부 SE를 무엇으로 붙일지와 그 SE가 secp256k1을 지원하는지(지원하지 않으면 SE는 래핑 비밀과 재시도 카운터만 맡고 서명은 MCU 소프트웨어로 합니다), 둘째, 판매자 결제 요청 서명 형식을 EIP-712 구조체로 할지 자체 형식으로 할지입니다. 이 두 결정이 1~2주차에 끝나지 않으면 보안 요구사항 4와 6이 뒤로 밀려 시연이 "또 하나의 BLE 지갑"으로 축소될 위험이 큽니다.
