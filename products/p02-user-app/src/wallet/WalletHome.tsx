import {useWalletConnectivity, type ReadProof} from './walletConnectivity.ts';
import {useCallback, useEffect, useEffectEvent, useLayoutEffect, useRef, useState, type ComponentRef} from 'react';
import {AccessibilityInfo, Alert, AppState, BackHandler, findNodeHandle, Image, Keyboard, Linking, Platform, ScrollView, Share, StyleSheet, Text, TextInput, View} from 'react-native';
import NativeCloudWalletProfile from '../specs/NativeCloudWalletProfile.ts';
import {CloudWalletScreen} from './cloud/CloudWalletScreen.tsx';
import {readCloudProfile, type CloudProfile} from './cloud/client.ts';
import Ble from '../specs/NativeRenterBle.ts';
import Http from '../specs/NativeCommerceHttp.ts';
import NativeWallet from '../specs/NativeOwnerWallet.ts';
import {english} from './walletEnglish.ts';
import {receiveQr} from './receivePresentation.ts';
import {ChainWalletScreen} from './ChainWalletScreen.tsx';
import {builtInEvmEnabled, defaultReadState, formatReadAmount, readProfiles} from './readProfiles.ts';
import {WalletHomeSummary} from './WalletHomeSummary.tsx';
import {WalletSettings} from './WalletSettings.tsx';
import {candidateTrustForInstallation} from './firmwareCandidate.ts';
import {firmwareCandidateDeployment} from './firmwareCandidateDeployment.ts';
import {WalletMaintenance} from './WalletMaintenance.tsx';
import {TravelPrivate} from '../travel/TravelPrivate.tsx';
import {maintenanceScope, sameMaintenanceScope} from './walletMaintenance.ts';
import {readOwnerFinalityFile, ownerLimitChangeApplied, type OwnerIdleLock, type OwnerLimits} from './ownerLimitsWire.ts';
import {readIssuedLogin} from './dapp/loginWire.ts';
import {DappBrowserScreen} from './dapp/DappBrowserScreen.tsx';
import {WalletConnectScreen,notifyStoredDisconnect,reviewAllowancePrompt,reviewPrompt} from './dapp/WalletConnectScreen.tsx';
import {verifyPeer,type VerifiedPeer} from './dapp/walletConnectPolicy.ts';
import {validateConnectCheckpoint,viewAllowanceConnection,type SessionOptions} from './dapp/walletConnectSession.ts';
import {OwnerSession, type LostPhoneContext, type OwnerDisplayBinding} from './ownerSession.ts';
import {bytesToHex,hexToBytes} from '@nu54/protocol';
import {fromBase64} from '@nu54/protocol';
import {authenticateRegistry,formatEvmTokenAmount,parseHumanAmount,readRegistryEntry} from './evmRegistry.ts';
import {checkEvmNetwork,finalizedAllowance,finalityMatchesPending,finalityMatchesSigning,storedAllowanceChange,evmProfiles,httpsEvmRpc,normalizeEvmAddress,queryEvmAllowanceChange,queryEvmTransfer,readDirectTransfer,readEvmReceiveBalance,verifyStoredEvmWire,verifyTransferAuthority,type ExternalSpendContext,type EvmChain} from './evmTransfer.ts';
import {consumeInstallation, installationTime, readInstallClock, readInstallTrust, startInstallation, type InstallPreflight} from './boardInstallation.ts';
import {beginPractice, practiceDecision, practiceOrderText, type Practice} from './practiceFlow.ts';
import {decodeWalletStorage, storedWalletConnects, testedBoard, withoutBoardAccount, type PendingEvmTransfer, type StoredConnect, type WalletStorage} from './profile.ts';
import {persistWalletSignature, WalletProfileStore} from './walletStorage.ts';
import {importRecoveryTrust} from './recoveryProof.ts';
import {BoardStepPanel} from './BoardStepPanel.tsx';
import {boardFailure, forgetReturn, pendingReset} from './boardReturn.ts';
import {DemoWalletHome} from './DemoWalletHome.tsx';
import type {OpenOrder} from '../merchant/orderRecords.ts';
import {BoardBalance, BoardLife, boardLifeTitles, SetupStatus, type BoardLifeScreen} from './BoardLife.tsx';
import {BoardCard, BoardPage, FirstSteps, HomeOrders} from './HomeParts.tsx';
import {Brand, Card, Chip, KitButton, Section, TabBar, TabHeader, type ButtonVariant} from '../ui/kit.tsx';
import {useScreenProtection} from '../ui/screenProtection.ts';
import {color, font} from '../ui/theme.ts';
import type {BoardWait} from './boardSteps.ts';
import {importEnrollment, OWNER_GATT, type CommissioningPins, type Enrollment, type OwnerStatus, type SpendingStatus} from './ownerWire.ts';

// The cloud wallet stays out of the menus in this release; its screens and code remain.
const showCloudWallet: boolean = false;

type RecoveryOrigin = 'profiles' | 'settings';
type RecoveryEnrollmentDraft = {document: string; device: string; fingerprint: string};
const emptyRecoveryEnrollment: RecoveryEnrollmentDraft = {document:'',device:'',fingerprint:''};

function Action({title, run, disabled = false, testID, variant = 'primary', detail}: {title: string; run: () => void; disabled?: boolean;
  testID?: string; variant?: ButtonVariant; detail?: string}) {
  return <KitButton testID={testID} label={title} run={run} disabled={disabled} variant={variant} detail={detail} />;
}

const innerPages: string[] = ['transfer', 'receipts', 'receive-evm', 'receive-chain', 'send-chain', 'read-profiles', 'evm-registry', 'firmware'];

function savedEnrollment(store: WalletStorage): Enrollment | undefined {
  const profile = store.enrollment;
  return profile && importEnrollment(JSON.stringify(profile), profile.device, profile.boardSPKI);
}

export function WalletHome({rental, orders, permission, honorExistingRental, onPracticeChange, commissioningPins, dismissCommissioningPins,
  initialExplore, onExploreRestored, exploreSite, onExploreSiteChange, openExploreTravel, openExploreMarket, services = [],
  tabbed = false, requestScreen, onScreenChange, openOrders = [], openOrder}: {
  rental: () => void; orders:()=>void; permission: () => Promise<boolean>; honorExistingRental: boolean;
  onPracticeChange?: (active: boolean) => void; commissioningPins?: CommissioningPins; dismissCommissioningPins?: () => void;
  initialExplore?:boolean; onExploreRestored?:()=>void; exploreSite?:string; onExploreSiteChange?:(site:string)=>void;
  openExploreTravel?:()=>void; openExploreMarket?:()=>void;
  /** App-level services (market, nearby shop, payments, travel) listed on the wallet's first screens. */
  services?: {label:string; run:()=>void}[];
  /** The app shell draws the bottom tabs; the wallet then leaves out its own settings and service entries. */
  tabbed?: boolean;
  /** Tab requests from the shell: the wallet home, settings or the DApp browser. A new nonce repeats a request. */
  requestScreen?: {screen:'home'|'board'|'owner'|'settings'|'dapp'|'allowance'|'walletconnect'; nonce:number};
  /** Orders in progress, kept current by the shell, and the way into one of them. */
  openOrders?: OpenOrder[]; openOrder?: (sale: string) => void;
  /** Reports the wallet's current screen so the shell can mark the active tab and hide tabs in secure flows. */
  onScreenChange?: (screen:string)=>void;
}) {
  const exploreOnLoad=useRef(initialExplore);
  const exploreBack=useRef<'allowance'|'walletconnect'|'evm-registry'|undefined>(undefined);
  const [language, setLanguage] = useState<"ko" | "en">("ko");
  const t = (ko: string) => language === "en" ? english[ko] ?? ko : ko;
  const [stored, setStored] = useState<WalletStorage>();
  const currentStorage=useRef<WalletStorage|undefined>(undefined);
  const storageQueue = useRef<Promise<unknown>>(Promise.resolve());
  const allowanceEpoch=useRef(0);
  const invalidateAllowance=useCallback(()=>{allowanceEpoch.current++;},[]);
  const allowanceForeground=useRef(AppState.currentState!=='background' && AppState.currentState!=='inactive');
  const [allowanceConnection,setAllowanceConnection]=useState<{origin:string;transport:'browser'|'walletconnect';chain:EvmChain;account:string}>();
  const [connectSelection,setConnectSelection]=useState<string|null>();
  const [,refreshConnections]=useState(0);
  const [allowanceSpender, setAllowanceSpender] = useState('');
  const [allowanceRead, setAllowanceRead] = useState<{scope:string; value?:bigint; activeHash?:string; result?:string}>();
  const [ownerPermissionDenied,setOwnerPermissionDenied]=useState(false);
  const [ownerScanComplete,setOwnerScanComplete]=useState(false);
  const [screen, setScreen] = useState<BoardLifeScreen | 'demo' | 'home' | 'board' | 'profiles' | 'cloud' | 'practice' | 'install' | 'settings' | 'settings-access' | 'settings-diagnostics' | 'settings-device' | 'policy-picker' | 'lock-picker' | 'credentials' | 'travel-privacy' | 'policy' | 'policy-applied' | 'policy-device' | 'owner' | 'recovery-options' | 'recovery-cloud-info' | 'recovery-cloud' | 'recovery' | 'backup' | 'replace-phone' | 'transfer' | 'allowance' | 'receipts' | 'receive-evm' | 'receive-chain' | 'send-chain' | 'read-profiles' | 'evm-registry' | 'dapp' | 'walletconnect' | 'firmware' | 'rental-maintenance' | 'rental-original-payment'>('profiles');
  const navigateRentalOriginal=useCallback((detail:boolean)=>setScreen(detail?'rental-original-payment':'rental-maintenance'),[]);
  const [cloudProfile,setCloudProfile] = useState<CloudProfile>();
  useEffect(()=>{
    let active=true;
    NativeCloudWalletProfile?.profile().then(raw=>{
      if(active&&raw)setCloudProfile(readCloudProfile(raw));
    }).catch(()=>undefined);
    return ()=>{active=false;};
  },[]);
  const [recoveryOrigin,setRecoveryOrigin] = useState<RecoveryOrigin>();
  const [recoveryEnrollmentDraft,setRecoveryEnrollmentDraft] = useState(emptyRecoveryEnrollment);
  const [recoveryEnrollment,setRecoveryEnrollment] = useState<Enrollment>();
  const cloudRecoveryAvailable = cloudProfile &&
    (!cloudProfile.onboarding || cloudProfile.onboarding.network==='sepolia-native-test-v1');

  function openRecoveryOptions(origin: RecoveryOrigin) {
    setRecoveryOrigin(origin);
    setRecoveryEnrollmentDraft(emptyRecoveryEnrollment);
    setRecoveryEnrollment(undefined);
    setMessage('');
    setScreen('recovery-options');
  }

  function chooseRecovery(route: 'replace-phone' | 'recovery' | 'recovery-cloud-info') {
    setRecoveryEnrollmentDraft(emptyRecoveryEnrollment);
    setRecoveryEnrollment(undefined);
    setFound([]);
    setMessage('');
    setScreen(route);
  }

  function leaveRecoveryOptions() {
    setRecoveryOrigin(undefined);
    setRecoveryEnrollmentDraft(emptyRecoveryEnrollment);
    setRecoveryEnrollment(undefined);
    setScreen(recoveryOrigin ?? 'profiles');
  }

  function returnToRecoveryOptions() {
    Keyboard.dismiss();
    if (screen==='recovery') {
      if (recoveryTarget || restore) endRecovery();
      setTrustDoc('');
      setPins(['','','']);
    }
    if (screen==='replace-phone' && lostPhone) closeLostPhone();
    setRecoveryEnrollmentDraft(emptyRecoveryEnrollment);
    setRecoveryEnrollment(undefined);
    setFound([]);
    setMessage('');
    setScreen('recovery-options');
  }
  const [practiceWarning, setPracticeWarning] = useState(false);
  const [installSetup,setInstallSetup] = useState('');
  const [installerPin,setInstallerPin] = useState('');
  const [candidatePin,setCandidatePin] = useState('');
  const [installDevice,setInstallDevice] = useState('');
  const [installReceipt,setInstallReceipt] = useState('');
  const [installAdvanced,setInstallAdvanced] = useState(false);
  const [installConfirmed,setInstallConfirmed] = useState(false);
  const [fromInstall,setFromInstall] = useState(false);
  const scroll = useRef<ComponentRef<typeof ScrollView>>(null);
  // Messages and board steps are drawn above the page; a button far below must bring them into view.
  const showTop=useCallback(()=>scroll.current?.scrollTo({y:0,animated:true}),[]);
  const resetAccessibilityScroll=useCallback(()=>scroll.current?.scrollTo({y:0,animated:false}),[]);
  const sectionHeading=useRef<ComponentRef<typeof Text>>(null);
  const previousSecure=useRef(false);
  const storedMode=stored?.mode;
  const storedVerified=!!stored?.verified;
  useEffect(() => {
    const secure=['backup','replace-phone','recovery','recovery-options','recovery-cloud-info','recovery-cloud'].includes(screen);
    // Navigation hides only in practice, installation and the secure recovery flows; the tabs stay
    // visible before enrollment so a first-time user can still reach payments and settings.
    onPracticeChange?.(screen === 'practice' || screen === 'install' || secure);
    scroll.current?.scrollTo({y: 0, animated: false});
    if (screen === 'practice') AccessibilityInfo.announceForAccessibility(language === 'ko' ? '결제 연습' : 'Payment practice');
    const shouldFocus=secure || (previousSecure.current && screen==='owner');previousSecure.current=secure;
    if(shouldFocus) {
      const heading=screen==='backup' ? (language==='ko'?'독립 백업':'Independent backup') :
        screen==='replace-phone' ? (language==='ko'?'분실한 휴대전화 교체':'Replace lost phone') :
        screen==='recovery' ? (language==='ko'?'지갑 복구':'Wallet recovery') :
        screen==='recovery-options' ? (language==='ko'?'어떤 기기를 교체하나요?':'Which device are you replacing?') :
        screen.startsWith('recovery-cloud') ? (language==='ko'?'클라우드 휴대전화 복구':'Cloud phone recovery') :
        (language==='ko'?'개인 지갑':'Personal wallet');
      AccessibilityInfo.announceForAccessibility(heading);
      const timer=setTimeout(()=>{const node=findNodeHandle(sectionHeading.current);if(node)AccessibilityInfo.setAccessibilityFocus(node);},200);
      return ()=>clearTimeout(timer);
    }
  }, [screen, onPracticeChange, language, fromInstall, storedMode, storedVerified]);
  const closeAllowance=useCallback(()=>{
    invalidateAllowance();
    setTransfer(undefined);
    setTransferWire('');
    setAllowanceRead(undefined);
    setScreen(exploreBack.current==='allowance'?'dapp':'owner');
    exploreBack.current=undefined;
  },[invalidateAllowance]);
  const returnToPolicyFromHardware = useEffectEvent(() => returnToPolicy());
  const recoverBackFromHardware = useEffectEvent(() => {
    if (operation.current) return;
    if (screen==='recovery-options') leaveRecoveryOptions();
    else if (screen==='recovery-cloud') setScreen('recovery-cloud-info');
    else returnToRecoveryOptions();
  });
  // Pages without their own way back return to the page that opened them.
  const innerFrom = useRef<typeof screen>('owner');
  useEffect(() => { if (!innerPages.includes(screen)) innerFrom.current = screen; }, [screen]);
  useEffect(() => {
    const home = tabbed && (screen === 'board' || (screen === 'owner' && storedMode === 'personal' && !fromInstall));
    const life = screen in boardLifeTitles;
    if (screen !== 'demo' && !home && !life) return;
    const listener = BackHandler.addEventListener('hardwareBackPress', () => {
      if (life) { setScreen(screen === 'operator-reset' ? 'reset' : 'board'); return true; }
      setScreen(!home ? 'owner' : storedMode === 'personal' ? 'home' : 'profiles'); return true;
    });
    return () => listener.remove();
  }, [screen, tabbed, storedMode, fromInstall]);
  useEffect(() => {
    if (screen==='dapp'||screen==='walletconnect'||screen==='evm-registry'&&exploreBack.current===screen) {
      const listener=BackHandler.addEventListener('hardwareBackPress',()=>{
        setScreen(exploreBack.current===screen?'dapp':'owner');
        exploreBack.current=undefined;
        return true;
      });
      return ()=>listener.remove();
    }
    if (screen === 'rental-original-payment') {
      const listener=BackHandler.addEventListener('hardwareBackPress',()=>{
        navigateRentalOriginal(false);
        return true;
      });
      return ()=>listener.remove();
    }
    if (recoveryOrigin && ['recovery-options','recovery-cloud-info','recovery','replace-phone'].includes(screen)) {
      const listener=BackHandler.addEventListener('hardwareBackPress',()=>{
        recoverBackFromHardware();
        return true;
      });
      return ()=>listener.remove();
    }
    if (screen === 'backup' || screen === 'replace-phone' || screen === 'recovery') {
      const listener=BackHandler.addEventListener('hardwareBackPress',()=>{
        if(screen==='backup')endBackup();else if(screen==='replace-phone')closeLostPhone();else endRecovery();
        setScreen('owner');return true;
      });
      return ()=>listener.remove();
    }
    if (['settings','settings-access','settings-diagnostics','settings-device','policy-picker','lock-picker','credentials','travel-privacy','policy','policy-applied','policy-device','allowance'].includes(screen)) {
      const listener = BackHandler.addEventListener('hardwareBackPress', () => {
        if (screen==='allowance') closeAllowance();
        else if(screen==='policy-device') returnToPolicyFromHardware();
        else setScreen(screen==='settings'?(tabbed?'home':'owner'):'settings');
        return true;
      });
      return () => listener.remove();
    }
    if (innerPages.includes(screen)) {
      const listener = BackHandler.addEventListener('hardwareBackPress', () => {
        if (operation.current) return true;
        setMessage('');
        setScreen(innerFrom.current);
        return true;
      });
      return () => listener.remove();
    }
    if (screen !== 'practice' && screen !== 'install' && !(screen === 'owner' && (fromInstall || stored?.mode !== 'personal'))) return;
    const listener = BackHandler.addEventListener('hardwareBackPress', () => {
      if (screen === 'owner') { setFromInstall(false); setScreen('profiles'); }
      else setScreen(stored?.mode === 'personal' && screen === 'practice' ? 'owner' : 'profiles');
      return true;
    });
    return () => listener.remove();
  }, [screen, stored?.mode, fromInstall, closeAllowance,navigateRentalOriginal,recoveryOrigin,tabbed]);
  const [message, setMessage] = useState('');
  const sayLife = useCallback((text: string) => { setMessage(text); if (text) showTop(); }, [showTop]);
  // The board has no screen: while a request waits on it, the app shows the board step and buttons.
  const [boardWait, setBoardWait] = useState<BoardWait>();
  useEffect(() => { if (boardWait) showTop(); }, [boardWait, showTop]);
  const appliedRequest = useRef(0);
  useEffect(() => {
    // The wallet may mount before its stored profile loads; apply the request once it has.
    if (!requestScreen || !stored || appliedRequest.current === requestScreen.nonce) return;
    appliedRequest.current = requestScreen.nonce;
    const verified = !!stored.verified;
    if (requestScreen.screen === 'home' && stored.mode !== 'personal') { setScreen('profiles'); return; }
    if (!['home', 'board', 'owner', 'settings'].includes(requestScreen.screen) && !verified) {
      setScreen('owner'); setMessage('보드를 연결하고 백업을 확인한 뒤 앱 연결을 사용할 수 있습니다.'); return;
    }
    if (requestScreen.screen === 'allowance') { openAllowance(); return; }
    if (requestScreen.screen === 'walletconnect') { openExternalConnection(connections[0]); return; }
    if (requestScreen.screen === 'dapp') dappSigned.current = undefined;
    setScreen(requestScreen.screen);
    // Only a new nonce, or the profile arriving, can apply a request.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [requestScreen?.nonce, !!stored]);
  useEffect(() => { onScreenChange?.(screen); }, [screen, onScreenChange]);
  // QA builds made with the demo switch can show the verified home with fixture data.
  const [demoAllowed, setDemoAllowed] = useState(false);
  const waitOnBoard = (wait: BoardWait, text: string) => {setMessage(text); setBoardWait(wait);};
  const [displayBinding,setDisplayBinding]=useState<OwnerDisplayBinding>();
  const [showDisplayBinding,setShowDisplayBinding]=useState(false);
  const displayCode=showDisplayBinding && displayBinding ?
    [displayBinding.operation,displayBinding.session,displayBinding.request,displayBinding.expires].join('/') : undefined;
  useEffect(()=>{
    if (!displayBinding) return;
    const timeout=setTimeout(()=>{
      setShowDisplayBinding(false);
      setDisplayBinding(undefined);
    },Math.max(0,Number(displayBinding.expires)*1000-Date.now()));
    return ()=>clearTimeout(timeout);
  },[displayBinding]);
  const [busy, setBusy] = useState(false);
  const [raw, setRaw] = useState('');
  const [board, setBoard] = useState('');
  const [pin, setPin] = useState('');
  const [publicKey, setPublicKey] = useState('');
  const [retiring, setRetiring] = useState(false);
  const [found, setFound] = useState<{address: string; name: string}[]>([]);
  const [trustDoc, setTrustDoc] = useState('');
  const [pins, setPins] = useState(['','','']);
  const [backup, setBackup] = useState<{requestId:string; expires:string}>();
  const [backupBlocked,setBackupBlocked]=useState(false);
  const [restore, setRestore] = useState<{challenge:string; requestId:string; expires:string}>();
  const [recoveryTarget,setRecoveryTarget]=useState<OwnerStatus>();
  const [lostPhone, setLostPhone] = useState<{handle:string; previous:LostPhoneContext; submitted:boolean}>();
  const [transferWire,setTransferWire] = useState('');
  const [transfer,setTransfer] = useState<ReturnType<typeof readDirectTransfer>>();
  const [rpcEndpoint,setRpcEndpoint] = useState('');
  const [evmReceiveBalance,setEvmReceiveBalance] = useState<{amount:string; block:string}>();
  const [rpcCandidate,setRpcCandidate] = useState<Record<string,string>>({});
  const [rpcOverride,setRpcOverride] = useState<Record<string,string>>({});
  const [registryEndpoint,setRegistryEndpoint] = useState('');
  const [registryToken,setRegistryToken] = useState('');
  const [evmAmount,setEvmAmount] = useState('');
  const [evmRecipient,setEvmRecipient] = useState('');
  const [evmAsset,setEvmAsset] = useState('');
  const [contactName,setContactName] = useState('');
  const [reclaimWire,setReclaimWire] = useState('');
  const [receiveChain,setReceiveChain] = useState<EvmChain>('8283');
  const [receiveAddress,setReceiveAddress] = useState('');
  const [homeActive, setHomeActive] = useState(AppState.currentState === 'active');
  const [policyView,setPolicyView] = useState<OwnerLimits|null>();
  const [policyFresh,setPolicyFresh] = useState(false);
  const [policyFinalityFresh,setPolicyFinalityFresh] = useState(false);
  const [policyFinalityPending,setPolicyFinalityPending] = useState(false);
  const [policyReadAt,setPolicyReadAt] = useState<number>();
  const [verifiedIdleLock,setVerifiedIdleLock] = useState<{board:string;generation:number;rule:OwnerIdleLock}>();
  const [policyRefreshRequired,setPolicyRefreshRequired] = useState(false);
  const [policyHandle,setPolicyHandle] = useState('');
  const [status, setStatus] = useState<OwnerStatus>();
  const [spending, setSpending] = useState<SpendingStatus>();
  // The board accepted Bluetooth but never finished the handshake: most likely no live operator permit.
  const [permitNeeded, setPermitNeeded] = useState(false);
  // When the account of this registration last proved itself on the connected board.
  const [seenBoard, setSeenBoard] = useState<{account: string; generation: number; at: number}>();
  const [profile, setProfile] = useState<Enrollment>();
  const recoveryBoard = recoveryOrigin ? recoveryEnrollment ?? (screen==='replace-phone' ? profile : undefined) : profile;
  const connection = useRef<OwnerSession>(undefined);
  const dappSigned=useRef<PendingEvmTransfer|undefined>(undefined);
  const operation = useRef(false);
  const native = NativeWallet;
  useEffect(() => {
    let live = true;
    // Older test doubles and builds without the method simply keep the demo off.
    if (typeof native?.demoWallet === 'function') native.demoWallet().then(value => { if (live) setDemoAllowed(value === true); }).catch(() => {});
    return () => { live = false; };
  }, [native]);
  const connectivity = useWalletConnectivity(native, stored?.mode === 'personal');
  const [readProof, setReadProof] = useState<ReadProof>();
  // The demo screen renders fixture data only, so it stays capturable for design review.
  const needsHomeProtection = stored?.mode === 'personal' && !['demo','profiles','cloud','practice','install','rental-maintenance','rental-original-payment'].includes(screen);
  const {on: homeProtected, failed: protectionError, retry: retryProtection} = useScreenProtection(needsHomeProtection, native);

  useEffect(() => {
    let mounted = true;
    const background = AppState.addEventListener('change', state => {
      setHomeActive(state === 'active');
      allowanceForeground.current=state==='active';
      if (state === 'active') return;
      setDisplayBinding(undefined);
      setShowDisplayBinding(false);
      invalidateAllowance();
      setAllowanceRead(undefined);
      setTransfer(previous=>previous?.revoke ? undefined : previous);
      if (connection.current) {
        connection.current.close();
        connection.current = undefined;
        setMessage('기기 연결을 닫았습니다. 계속하려면 다시 인증하세요.');
      }
      setRestore(undefined);
      setRecoveryTarget(undefined);
      setBackup(undefined);
      setLostPhone(undefined);
      setReceiveAddress(''); setEvmReceiveBalance(undefined);
      setStatus(undefined);
      setPolicyView(undefined); setPolicyFresh(false); setPolicyFinalityFresh(false); setPolicyFinalityPending(false); setPolicyHandle(''); setPolicyReadAt(undefined); setPolicyRefreshRequired(false); setVerifiedIdleLock(undefined);
    });
    (async () => {
      const storedText = native ? await native.loadProfile() : '';
      const initial = decodeWalletStorage(storedText);
      const enrollment = savedEnrollment(initial);
      const v1 = Platform.OS === 'android' ? JSON.parse((await Ble.loadDevices()) || '[]') : [];
      if (!mounted) return;
      currentStorage.current=initial;
      setStored(initial);
      setFromInstall(initial.installation?.state === 'consumed' && initial.mode === 'personal');
      setLanguage(initial.language ?? 'ko');
      setProfile(enrollment);
      if (initial.mode === 'personal') {
        setScreen(initial.phoneReplacement ? 'replace-phone' : initial.verified && exploreOnLoad.current ? 'dapp' : tabbed ? 'home' : 'owner');
      }
      if (exploreOnLoad.current) {exploreOnLoad.current=false;onExploreRestored?.();}
      if (initial.mode !== 'personal' && honorExistingRental && Platform.OS === 'android' && (initial.mode === 'legacy' || (Array.isArray(v1) && v1.length))) rental();
    })().catch(() => {
      if (mounted) setMessage('보호된 지갑 기록을 열 수 없습니다. 기록을 지우거나 기존 대여로 전환하기 전에 지원을 요청하세요.');
    });
    return () => {
      mounted = false;
      background.remove();
      connection.current?.close();
      connection.current = undefined;
    };
  // The shell decides once whether it draws the tabs.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [native, rental, honorExistingRental, invalidateAllowance, onExploreRestored]);

  function work(run: () => Promise<void>, failure: string | ((error: unknown) => string)) {
    if (AppState.currentState === 'background' || operation.current) return;
    operation.current = true;
    setBusy(true);
    setMessage('');
    setBoardWait(undefined);
    run().catch(error => { setMessage(typeof failure === 'string' ? failure : failure(error)); showTop(); }).finally(() => {
      operation.current = false;
      setBusy(false);
      setBoardWait(undefined);
    });
  }

  async function commit(next:WalletStorage|((head:WalletStorage)=>WalletStorage), base = stored) {
    if (!native || !base) throw new Error('NATIVE_PROFILE_REQUIRED');
    const write = storageQueue.current.then(async () => {
      const previous=typeof next==='function' ? currentStorage.current : base;
      if (!previous) throw new Error('WALLET_STORAGE');
      let result=typeof next==='function' ? next(previous) : next;
      const owner=result.verified;
      if (storedWalletConnects(result).some(({checkpoint:{scope}})=>!result.evmRegistry ||
          scope.board!==owner?.boardFingerprint || scope.generation!==owner?.generation || scope.account!==owner?.address))
        result={...result,walletConnect:undefined,walletConnectSessions:undefined};
      const committed = await new WalletProfileStore(native).save(previous,result);
      currentStorage.current = committed;
      setStored(committed);
      return committed;
    });
    storageQueue.current = write.catch(() => undefined);
    return write;
  }

  async function save(next: WalletStorage, base = stored) {
    await commit(next, base);
  }

  function scan(readOnly = false, scopedRecovery = false) {
    work(async () => {
      if (!native || !stored) throw new Error('PROFILE_REQUIRED');
      setOwnerScanComplete(false);
      if (!await permission()) {
        setFound([]);
        if (Platform.OS !== 'android') throw new Error('BLUETOOTH_PERMISSION');
        setOwnerPermissionDenied(true);
        setMessage(t('주변 기기 권한이 필요합니다. 앱 설정에서 허용하고 돌아와 검색을 다시 시도하세요.'));
        return;
      }
      setOwnerPermissionDenied(false);
      if(readOnly && (!stored.verified || profile?.device!==stored.verified.boardFingerprint)) throw new Error('PROFILE_REQUIRED');
      const selected = scopedRecovery
        ? recoveryBoard ?? importEnrollment(recoveryEnrollmentDraft.document,
            recoveryEnrollmentDraft.device.trim(),recoveryEnrollmentDraft.fingerprint.trim())
        : profile ?? importEnrollment(raw, board.trim(), pin.trim());
      if (!scopedRecovery && fromInstall && (stored.installation?.state !== 'consumed' || selected.device !== stored.installation.claim.device))
        throw new Error('INSTALL_BOARD_MISMATCH');
      // Persist the mode and certificates before sending a mutation. A restart
      // can resume with a fresh authenticated STATUS if the reply was lost.
      const sameBoard = stored.enrollment?.device === selected.device;
      if(scopedRecovery) {
        setRecoveryEnrollment(selected);
        setRecoveryEnrollmentDraft(emptyRecoveryEnrollment);
      } else if(!readOnly) {
        await save({...stored,mode:'personal',enrollment:selected,backupCheckpoint:sameBoard ? stored.backupCheckpoint : undefined,
          evmRegistry:sameBoard ? stored.evmRegistry : undefined});
        setProfile(selected);
        setRaw('');
      }
      setFound([]);
      const listener = Ble.onScan(value => {
        try {
          const item = JSON.parse(value);
          if (typeof item.address !== 'string' || item.address.length > 64 || typeof item.name !== 'string') return;
          setFound(previous => previous.some(entry => entry.address === item.address) ? previous : [...previous, item]);
        } catch {
          // Raw BLE advertisements cannot alter the wallet state.
        }
      });
      try {
        await Ble.scanOwner(OWNER_GATT.service, 6);
      } finally {
        listener.remove();
      }
      setOwnerScanComplete(true);
      setMessage('기기 목록에서 선택하세요. 앱이 등록서에 지정된 기기인지 확인합니다.');
    }, '등록 정보나 블루투스 권한을 확인하고 다시 검색하세요.');
  }

  async function maintenanceRequest<T>(handle:string, run:(session:OwnerSession)=>Promise<T>) {
    const scope=maintenanceScope(currentStorage.current);
    if(AppState.currentState!=='active'||!native||!profile||scope.mode!=='personal'||profile.device!==scope.board||
        currentStorage.current?.signing||connection.current)throw new Error('MAINTENANCE_SCOPE');
    const session=new OwnerSession(Ble,native,profile,!!currentStorage.current?.spendingBoard);
    connection.current=session;
    try{
      await session.connect(handle);
      const current=()=>AppState.currentState==='active'&&connection.current===session&&sameMaintenanceScope(scope,maintenanceScope(currentStorage.current))&&!currentStorage.current?.signing;
      if(!current())throw new Error('MAINTENANCE_SCOPE');
      const result=await run(session);
      if(!current())throw new Error('MAINTENANCE_SCOPE');
      return result;
    }finally{
      if(connection.current===session){
        connection.current=undefined;
        session.close();
      }
    }
  }

  /** One authenticated session with the enrolled board, whatever its owner state is. */
  async function boardRequest<T>(handle: string, run: (session: OwnerSession) => Promise<T>) {
    if (!native || !profile || connection.current || currentStorage.current?.enrollment?.device !== profile.device) throw new Error('PROFILE_REQUIRED');
    const session = new OwnerSession(Ble, native, profile);
    connection.current = session;
    try {
      await connectBoard(session, handle);
      await forgetResetBoard(await session.status());
      const result = await run(session);
      if (connection.current !== session) throw new Error('OWNER_DISCONNECTED');
      return result;
    } finally {
      if (connection.current === session) {
        connection.current = undefined;
        session.close();
      }
    }
  }

  async function forgetBoardAccount() {
    if (Http) { await forgetReturn(Http); await pendingReset(Http).save(undefined); }
    await commit(withoutBoardAccount);
    setSeenBoard(undefined); setSpending(undefined); setVerifiedIdleLock(undefined); setPolicyHandle('');
  }

  /** A reset or a new registration destroyed the key this phone remembers; its address must not be shown again. */
  async function forgetResetBoard(confirmed: OwnerStatus) {
    const head = currentStorage.current, known = head?.verified;
    if (!known || head.phoneReplacement || known.boardFingerprint !== confirmed.device) return;
    // A personal wallet keeps its account when the phone is replaced; a spending wallet never does.
    if (confirmed.state !== 'released' && confirmed.state !== 'new' &&
        (confirmed.root === 2 || confirmed.generation === BigInt(known.generation))) return;
    await forgetBoardAccount();
  }

  async function connectBoard(session: OwnerSession, handle: string) {
    try {
      await session.connect(handle);
      setPermitNeeded(false);
    } catch (error) {
      if (session.reached !== 'bluetooth') throw error;
      setPermitNeeded(true);
      throw new Error('BOARD_PERMIT');
    }
  }

  async function saveBackedUpAddress(session: OwnerSession, confirmed: OwnerStatus, address: string) {
    if (!stored || stored.enrollment?.device !== confirmed.device || connection.current !== session ||
        confirmed.generation <= 0n || confirmed.generation > BigInt(Number.MAX_SAFE_INTEGER)) {
      throw new Error('OWNER_BACKUP_ADDRESS_BINDING');
    }
    const generation = Number(confirmed.generation);
    const existing = currentStorage.current?.verified;
    if (existing) {
      if (existing.boardFingerprint !== confirmed.device || existing.address !== address ||
          existing.generation !== generation || existing.network !== '8283') throw new Error('ACCOUNT_CHANGED');
      return;
    }
    await commit(head => ({...head,backupCheckpoint:undefined,verified:{boardFingerprint:confirmed.device,address,generation,network:'8283'}}));
  }

  function open(handle: string, action: 'register' | 'status' | 'revoke') {
    work(async () => {
      if (!native || !profile) throw new Error('PROFILE_REQUIRED');
      setStatus(undefined);
      setSpending(undefined);
      connection.current?.close();
      const session = new OwnerSession(Ble, native, profile);
      connection.current = session;
      try {
        await connectBoard(session, handle);
        const identity = await session.identity();
        if (fromInstall && action === 'register') {
          const checkpoint=stored?.installation;
          const current=await session.status();
          if (!checkpoint || checkpoint.state !== 'consumed') throw new Error('INSTALL_PROOF_STALE');
          const now=installationTime(checkpoint.clock,readInstallClock(await native.installationClock()));
          if (now < checkpoint.expected.now ||
              now >= checkpoint.claim.expiresAt || profile.device !== checkpoint.claim.device ||
              !((current.state === 'new' && current.generation === BigInt(checkpoint.claim.generation)) ||
                (current.state === 'active' && current.generation === BigInt(checkpoint.claim.generation + 1))))
            throw new Error('INSTALL_PROOF_STALE');
        }
        const confirm = () => waitOnBoard({flow:'register',step:'approve',note:'길게 누르면 승인으로 처리되지 않습니다. 이어서 보드 버튼으로 PIN을 정합니다.'},'보드 버튼으로 이 휴대전화의 등록을 승인하거나 거절하세요.');
        const before = action === 'register' ? await session.status() : undefined;
        const starting = before?.state === 'new' || before?.state === 'released';
        if (starting && (currentStorage.current?.verified || currentStorage.current?.spendingBoard)) await forgetBoardAccount();
        const confirmed = action === 'register' ? await session.register(confirm)
          : action === 'revoke' ? await session.revoke(confirm) : await session.status();
        if (confirmed.state === 'active' && confirmed.owner !== identity.fingerprint) throw new Error('OWNER_MISMATCH');
        const former = currentStorage.current?.verified;
        if (starting && former) await forgetBoardAccount(); // whatever was written while the board was asked
        else await forgetResetBoard(confirmed);
        const forgotten = !!former && !currentStorage.current?.verified;
        const backupCheckpoint = stored?.backupCheckpoint;
        if (action === 'status' && confirmed.state === 'active' && confirmed.root === 2 &&
            confirmed.profile === 1 && confirmed.backedUp && !stored?.verified &&
            backupCheckpoint?.device === confirmed.device && backupCheckpoint.owner === confirmed.owner &&
            backupCheckpoint.generation === confirmed.generation.toString()) {
          await saveBackedUpAddress(session, confirmed, await session.backedUpAddress(confirmed));
        }
        // A spending board has no backup: its account passes a one-time signature test instead.
        if (action === 'status' && confirmed.state === 'active' && confirmed.root !== 2) {
          try {
            const known = currentStorage.current?.verified;
            const account = await session.spendingAccount(confirmed);
            if (!known) await saveBackedUpAddress(session, confirmed, account);
            setSpending(await session.spendingStatus(confirmed.generation));
            setSeenBoard(!known || known.address === account ? {account, generation: Number(confirmed.generation), at: Date.now()} : undefined);
          } catch { /* Another firmware profile ends the session; its own checks above still apply. */ }
        }
        setStatus(confirmed);
        const verified=currentStorage.current?.verified;
        if(confirmed.state==='active' && confirmed.device===verified?.boardFingerprint &&
           confirmed.generation===BigInt(verified.generation))setPolicyHandle(handle);
        if (confirmed.state === 'released') {
          setMessage(forgotten ? '보드가 초기화되어 있어 이 휴대전화의 이전 보드 정보를 지웠습니다. 이전 주소로 보내지 말고 다시 등록하세요.' : '초기화된 보드입니다. 다시 등록할 수 있습니다.');
          return;
        }
        if (confirmed.state === 'revoked') {
          setPolicyHandle('');setVerifiedIdleLock(undefined);
          setRetiring(false);
          setMessage('기기가 이 휴대전화의 관리 권한을 철회했습니다. 자산 키는 기기에 보존됩니다.');
          return;
        }
        setMessage(forgotten ? '보드가 다시 등록되어 이전 보드 정보를 지웠습니다. 서명 시험을 다시 하세요.' : confirmed.state === 'active'
          ? '이 휴대전화의 소유자 등록을 기기에서 확인했습니다.'
          : '기기 상태를 확인했습니다. 새 기기라면 등록을 시작할 수 있습니다.');
        showTop();
      } finally {
        if (connection.current === session) {
          session.close();
          connection.current = undefined;
        }
      }
    }, error => error instanceof Error && error.message === 'BOARD_PERMIT' ? boardFailure(error) : '기기 확인 결과를 받지 못했습니다. 다시 연결해 등록 상태를 확인하세요.');
  }

  function closeLostPhone() {
    connection.current?.close();
    connection.current = undefined;
    setLostPhone(undefined);
  }

  function startLostPhone(handle: string) {
    work(async () => {
      if (!native || !recoveryBoard) throw new Error('PROFILE_REQUIRED');
      closeLostPhone();
      const session = new OwnerSession(Ble,native,recoveryBoard);
      connection.current = session;
      try {
        await session.connect(handle);
        const previous = await session.beginLostPhone();
        setLostPhone({handle,previous,submitted:false});
        setMessage(t('독립 복구 장치에 요청을 전달할 준비를 하세요.'));
      } catch (error) {
        closeLostPhone();
        throw error;
      }
    },t('원래 보드에서 SW3을 길게 누른 뒤 다시 연결하세요.'));
  }

  function submitLostPhone() {
    const pending = lostPhone;
    const session = connection.current;
    if (!pending || !session || !stored || !recoveryBoard) return;
    work(async () => {
      try {
        const {device,replacement,generation,address} = pending.previous;
        await save({...stored,mode:'personal',enrollment:recoveryBoard,
          phoneReplacement:{device,replacement,generation:generation.toString(),address}});
        setLostPhone({...pending,submitted:true});
        await session.proveLostPhone(() => waitOnBoard({flow:'replace',step:'recoveryproof'},t('등록된 독립 장치에서 확인한 뒤 보드에서 SW1을 누르고 PIN을 입력하세요.')));
        setMessage(t('보드에 다시 연결해 새 소유자와 같은 지갑 주소를 확인하세요.'));
      } finally {
        session.close();
        connection.current = undefined;
      }
    },t('결과가 불확실합니다. 교체를 다시 요청하기 전에 보드 상태를 확인하세요.'));
  }

  function verifyLostPhone(handle:string) {
    const checkpoint = stored?.phoneReplacement;
    if (!checkpoint || !stored) return;
    work(async () => {
      if (!native || !recoveryBoard) throw new Error('PROFILE_REQUIRED');
      const session = new OwnerSession(Ble,native,recoveryBoard);
      connection.current = session;
      try {
        await session.connect(handle);
        const checked = await session.verifyLostPhone(checkpoint);
        const previous = stored.verified;
        const ready = checked.root === 2 && checked.profile === 1 && checked.backedUp &&
          checked.generation <= BigInt(Number.MAX_SAFE_INTEGER);
        if (previous && (!ready || previous.boardFingerprint !== checkpoint.device ||
            previous.address !== checkpoint.address || previous.generation.toString() !== checkpoint.generation)) {
          throw new Error('ACCOUNT_CHANGED');
        }
        const verified = ready ? {boardFingerprint:checkpoint.device,address:checkpoint.address,
          generation:Number(checked.generation),network:'8283' as const} : undefined;
        await save({...stored,phoneReplacement:undefined,verified,pendingChain:stored.pendingChain ? {...stored.pendingChain,attempted:true} : undefined,evmRegistry:undefined});
        setStatus(checked);
        setLostPhone(undefined);
        setProfile(recoveryBoard);
        setRecoveryOrigin(undefined);
        setScreen('owner');
        setMessage(t('새 소유자 세대와 그대로 유지된 지갑 주소를 확인했습니다.') + ' ' +
          t(checked.legacyBondsClear
            ? '보드에 남은 이전 휴대전화의 BLE 연결 기록이 없습니다.'
            : '이전 휴대전화의 BLE 연결 기록은 아직 확인하지 못했습니다.'));
      } catch (error) {
        if (!(error instanceof Error) || error.message !== 'OWNER_REPLACEMENT_NOT_COMMITTED') throw error;
        await save({...stored,phoneReplacement:undefined});
        setLostPhone(undefined);
        setMessage(t('보드에 기존 소유자가 남아 있습니다. 교체를 다시 요청할 수 있습니다.'));
      } finally {
        session.close();
        connection.current = undefined;
      }
    },t('새 소유자와 기존 주소를 아직 확인하지 못했습니다.'));
  }

  async function chainSession<T>(handle: string, action: (active: OwnerSession) => Promise<T>): Promise<T> {
    if (!native || !profile || !stored?.verified || stored.verified.boardFingerprint !== profile.device)
      throw new Error('CHAIN_ACCOUNT_BINDING');
    const session = new OwnerSession(Ble, native, profile);
    connection.current = session;
    try {
      await session.connect(handle);
      return await action(session);
    } finally {
      setShowDisplayBinding(false);
      setDisplayBinding(undefined);
      session.close();
      connection.current = undefined;
    }
  }

  const selectedRead=stored?.walletRead?.selection;
  const freshPolicyAsset=stored?.verified && selectedRead?.id==='stable' && selectedRead.asset ?
    stored.walletRead?.assets?.find(asset=>asset.board===stored.verified?.boardFingerprint &&
      asset.generation===stored.verified.generation && asset.id==='stable' && asset.account===selectedRead.account &&
      asset.contract===selectedRead.asset && !asset.hidden) : undefined;

  const policyDecimals=policyView ? stored?.walletRead?.assets?.find(asset => asset.board===policyView.board &&
    asset.generation===Number(policyView.generation) && asset.contract===policyView.asset &&
    ((asset.id==='stable' && policyView.chain===8283n) || (asset.id==='eth' && policyView.chain===11155111n) ||
     (asset.id==='base' && policyView.chain===84532n)))?.decimals : undefined;
  const policyPrompt = (binding:OwnerDisplayBinding) => {
    setDisplayBinding(binding);
    waitOnBoard({flow:'sign',step:'display'},language === 'ko' ? '신뢰하는 독립 표시에만 요청 정보를 전달하고, 확인한 뒤 보드에서 승인하세요.' :
      'Give the request binding only to your trusted independent display, then confirm on the board.');
  };
  function initializePolicy(destination:string,per:string,daily:string) {
    if (!freshPolicyAsset || !policyHandle) return;
    work(async()=>{
      const a=parseHumanAmount(per,freshPolicyAsset.decimals), b=parseHumanAmount(daily,freshPolicyAsset.decimals);
      await chainSession(policyHandle,session=>session.initializeOwnerLimits(BigInt(stored!.verified!.generation),'8283',freshPolicyAsset.contract,destination,a.toString(),b.toString(),policyPrompt));
      setPolicyFresh(false);setPolicyView(undefined);
      setMessage(language === 'ko' ? '기기 기록을 다시 읽어 설정을 확인하세요.' : 'Read the device policy again to verify the result.');
    },language === 'ko' ? '설정을 확인하지 못했습니다. 기기 기록을 다시 읽으세요.' : 'Could not confirm setup. Read the device policy again.');
  }
  function changePolicy(destination:string,per:string,daily:string) {
    if (!policyView || policyDecimals===undefined || !policyHandle) return;
    const expected=policyView;
    work(async()=>{
      const a=parseHumanAmount(per,policyDecimals), b=parseHumanAmount(daily,policyDecimals);
      setPolicyRefreshRequired(true);
      await chainSession(policyHandle,session=>session.requestOwnerLimitChange(expected,a.toString(),b.toString(),destination,policyPrompt));
      setMessage(language === 'ko' ? '기기 기록을 다시 읽어 대기 상태를 확인하세요.' : 'Read the device policy again to verify the pending change.');
    },language === 'ko' ? '변경 결과를 확인하지 못했습니다. 기기 기록을 다시 읽으세요.' : 'Could not confirm the change. Read the device policy again.');
  }
  function changeSetPolicy(change:{recipients:string[]}|{per:string;daily:string}) {
    if (!policyView?.recipients || !policyHandle) return;
    const expected=policyView;
    work(async()=>{
      const proposal='recipients' in change ? change : policyDecimals===undefined ? undefined :
        {per:parseHumanAmount(change.per,policyDecimals),daily:parseHumanAmount(change.daily,policyDecimals)};
      if(!proposal)throw new Error('OWNER_LIMITS_ASSET');
      setPolicyRefreshRequired(true);
      await chainSession(policyHandle,session=>session.requestOwnerSetChange(expected,proposal,policyPrompt));
      setMessage(language==='ko'?'기기 기록을 다시 읽어 대기 상태를 확인하세요.':'Read the device policy again to verify the pending change.');
    },language==='ko'?'변경 결과를 확인하지 못했습니다. 기기 기록을 다시 읽으세요.':'Could not confirm the change. Read the device policy again.');
  }
  const policyAppliedMessage=language==='ko'?'기기에서 새 활성 정책을 확인했습니다.':
    'The device confirmed the new active policy.';
  function refreshPolicy(handle:string) {
    work(async()=>{
      const generation=BigInt(stored!.verified!.generation);
      const set=await chainSession(handle,session=>session.ownerLimitsSet(generation)).catch(error=>{
        if(error instanceof Error && error.message==='OWNER_DISCONNECTED')return undefined;
        throw error;
      });
      const value=set??await chainSession(handle,session=>session.ownerLimits(generation));
      setPolicyFresh(value==='fresh');
      setPolicyFinalityFresh(value==='finality-fresh');
      setPolicyFinalityPending(value==='finality-pending');
      setPolicyView(typeof value==='string'?null:value??null);
      setPolicyReadAt(Date.now());
      setPolicyHandle(handle);
      setPolicyRefreshRequired(false);
      const applied=policyView && value && typeof value!=='string' && ownerLimitChangeApplied(policyView,value);
      setScreen(applied?'policy-applied':'policy');
      if(applied)setMessage(policyAppliedMessage);
    },language==='ko'?'기기의 보호 정책을 확인하지 못했습니다.':'Could not verify the device policy.');
  }

  function checkOwnerIdleLock(handle:string, returnToSettings=false) {
    const owner=stored?.verified;
    if (!owner) return;
    work(async()=>{
      setVerifiedIdleLock(undefined);
      const rule=await chainSession(handle,session=>session.ownerIdleLock(BigInt(owner.generation)));
      const current=currentStorage.current?.verified;
      if(AppState.currentState==='background' || AppState.currentState==='inactive' ||
         current?.boardFingerprint!==owner.boardFingerprint || current.generation!==owner.generation)return;
      setVerifiedIdleLock({board:owner.boardFingerprint,generation:owner.generation,rule});
      setPolicyHandle(handle);
      if(returnToSettings)setScreen('settings');
    },language==='ko'?'이 기기의 자동 잠금 규칙을 확인하지 못했습니다.':'Could not verify this device’s auto-lock rule.');
  }

  function openSettings() {
    setMessage('');
    setScreen('settings');
    const owner=stored?.verified;
    if(policyHandle && owner &&
       (verifiedIdleLock?.board!==owner.boardFingerprint || verifiedIdleLock.generation!==owner.generation))
      checkOwnerIdleLock(policyHandle);
  }

  function openDeviceVerification(kind:'policy'|'lock') {
    if(policyHandle) {
      if(kind==='policy')refreshPolicy(policyHandle);
      else checkOwnerIdleLock(policyHandle);
      return;
    }
    setFound([]);
    setMessage('');
    setScreen(kind==='policy'?'policy-picker':'lock-picker');
    scan(true);
  }

  function returnToPolicy() {
    setScreen('policy');
    if (policyHandle) refreshPolicy(policyHandle);
  }

  function activatePolicy() {
    if (!policyHandle || !policyView?.pending) return;
    const expected=policyView;
    work(async()=>{
      const {value, rule}=await chainSession(policyHandle,async session=>{
        const active=expected.recipients ? await session.activateOwnerSet(expected) : await session.activateOwnerLimitChange(expected);
        const capability=await session.ownerIdleLock(active.generation).catch(()=>undefined);
        return {value:active,rule:capability};
      });
      const owner=currentStorage.current?.verified;
      if(AppState.currentState==='background' || AppState.currentState==='inactive' ||
         owner?.boardFingerprint!==value.board || BigInt(owner.generation)!==value.generation)return;
      setVerifiedIdleLock(rule ? {board:value.board,generation:owner.generation,rule} : undefined);
      setPolicyView(value);
      setPolicyReadAt(Date.now());
      setScreen('policy-applied');
      setMessage(policyAppliedMessage);
    },language==='ko'?'변경 적용을 확인하지 못했습니다. 기기에서 보호 정책을 다시 읽으세요.':
      'Could not confirm activation. Read the device policy again.');
  }

  function finalityFile(kind:'enrollment'|'evidence') {
    if(!policyHandle || !native || !stored?.verified)return;
    work(async()=>{
      const file=readOwnerFinalityFile(await native.pickInstallationFile());
      if(kind==='enrollment' ? file.kind!=='enrollment' : file.kind==='enrollment')throw new Error('OWNER_FINALITY_FILE');
      const generation=BigInt(stored.verified!.generation);
      if(file.kind==='enrollment')await chainSession(policyHandle,session=>session.enrollOwnerFinality(generation,file.grant,file.signature,policyPrompt));
      else if(file.kind==='replacement')await chainSession(policyHandle,session=>session.reconcileOwnerReplacement(generation,file.body,file.signature,file.raw,file.replacement));
      else await chainSession(policyHandle,session=>session.reconcileOwnerFinality(generation,file.body,file.signature,file.raw));
      if(file.kind!=='enrollment') {
        const current=currentStorage.current;
        if(current) {
          const pendingMatches=!!current.pendingEvm && finalityMatchesPending(file.body,file.raw,current.pendingEvm);
          const signingMatches=!!current.signing && current.signing.kind!=='bip48' && finalityMatchesSigning(file.body,file.raw,{...current.signing,kind:current.signing.kind});
          if(pendingMatches || signingMatches)await save({...current,...(pendingMatches?{pendingEvm:undefined}:{}),...(signingMatches?{signing:undefined}:{})},current);
        }
      }
      const value=await chainSession(policyHandle,session=>session.ownerLimits(generation));
      setPolicyFresh(value==='fresh');setPolicyFinalityFresh(value==='finality-fresh');setPolicyFinalityPending(value==='finality-pending');
      setPolicyView(typeof value==='string'?null:value??null);setPolicyReadAt(Date.now());
      setMessage(language==='ko'?'기기에서 처리 결과를 확인했습니다.':'The device confirmed the result.');
    },language==='ko'?'처리 결과를 확인하지 못했습니다. 기기 상태를 다시 읽고 재개하세요.':'Could not confirm the result. Read the device status and resume.');
  }
  function resumeFinality() {
    if(!policyHandle || !stored?.verified)return;
    work(async()=>{
      const generation=BigInt(stored.verified!.generation);
      await chainSession(policyHandle,session=>session.resumeOwnerFinality(generation));
      const value=await chainSession(policyHandle,session=>session.ownerLimits(generation));
      setPolicyFinalityPending(value==='finality-pending');setPolicyFinalityFresh(value==='finality-fresh');
      setPolicyView(typeof value==='string'?null:value??null);setPolicyReadAt(Date.now());
    },language==='ko'?'설정을 재개하지 못했습니다. 기기 상태를 다시 확인하세요.':'Could not resume setup. Check the device status.');
  }

  const registry = stored?.evmRegistry ? readRegistryEntry(stored.evmRegistry).registry : undefined;
  const visibleEvmNetworks = registry?.networks.filter(network => builtInEvmEnabled(network.chain, stored?.walletRead?.disabled)) ?? [];
  const chosenNetwork = visibleEvmNetworks.find(network=>network.chain===receiveChain) ?? visibleEvmNetworks[0];
  const chosenAsset = chosenNetwork?.assets.find(asset=>asset.address===evmAsset) ?? chosenNetwork?.assets[0];
  const rpcKey = (chain: EvmChain, genesis: string) => [stored?.verified?.boardFingerprint, registry?.leaseRevision, chain, genesis].join('/');
  const selectedRpc = (network: {chain: EvmChain; genesis: string; rpc: string}) =>
    rpcOverride[rpcKey(network.chain, network.genesis)] ?? network.rpc;
  const networkFailure = (fallback: string) => (error: unknown) => t(
    error instanceof Error && error.message === 'EVM_RPC_GENESIS_UNAVAILABLE' ?
      'RPC에서 제네시스 블록을 조회할 수 없습니다. 네트워크 설정에서 대체 RPC를 직접 확인하세요.' : fallback);
  const registryCurrent = !!registry && registry.expires>Date.now()/1000;
  function currentCommissioned(key?:Uint8Array, revision?:bigint) {
    const entry=stored?.evmRegistry, owner=stored?.verified;
    if (!entry?.anchor || !owner || !registryCurrent || !registry) return [];
    return authenticateRegistry(entry,key ?? hexToBytes(entry.anchor),revision ?? BigInt(registry.leaseRevision),
      {device:owner.boardFingerprint,address:owner.address,generation:owner.generation},Math.floor(Date.now()/1000)).registry.chainNetworks ?? [];
  }
  const commissioned=(() => {try {return currentCommissioned();} catch {return [];}})();
  const pendingEvmReview = (() => {
    if ((screen !== 'transfer' && screen !== 'allowance') || !stored?.pendingEvm) return undefined;
    const {raw,hash,chain}=stored.pendingEvm;
    try {return verifyStoredEvmWire(raw,hash,chain);} catch {return undefined;}
  })();

  const pendingAllowance = (() => {
    const pending=stored?.pendingEvm;
    const owner=stored?.verified;
    if (!pending || !pendingEvmReview || !pending.genesis || !owner || pending.sender!==owner.address ||
        (pending.evidence && (pending.evidence.board!==owner.boardFingerprint || pending.evidence.generation!==owner.generation))) return undefined;
    try {return storedAllowanceChange(pending.raw);} catch {return undefined;}
  })();
  const allowanceScope = (chain:string, genesis:string, token:string, spender:string) => JSON.stringify([
    stored?.verified?.boardFingerprint, stored?.verified?.address, stored?.verified?.generation, chain, genesis, token, spender,
  ]);
  const activeAllowanceScope = (() => {
    if (pendingAllowance && stored?.pendingEvm?.genesis)
      return allowanceScope(stored.pendingEvm.chain,stored.pendingEvm.genesis,pendingAllowance.token,pendingAllowance.spender);
    if (!registryCurrent || !chosenNetwork || !chosenAsset) return undefined;
    try {return allowanceScope(chosenNetwork.chain,chosenNetwork.genesis,chosenAsset.address,normalizeEvmAddress(allowanceSpender.trim()));}
    catch {return undefined;}
  })();
  const pendingApproval = pendingAllowance && pendingAllowance.amount>0n;
  const allowanceHash=stored?.pendingEvm?.hash;
  const allowanceScreen=screen==='allowance';
  useLayoutEffect(()=>{
    if (!allowanceScreen) return;
    return invalidateAllowance;
  },[allowanceScreen,activeAllowanceScope,allowanceHash,invalidateAllowance]);
  const keepAllowanceResult = (epoch:number) => allowanceForeground.current && allowanceEpoch.current===epoch;
  const allowance = allowanceRead?.scope===activeAllowanceScope ? allowanceRead : undefined;
  const savedConnections=storedWalletConnects(stored);
  const connections=savedConnections.filter(connection=>{
    const owner=stored?.verified, {scope}=connection.checkpoint;
    const network=visibleEvmNetworks.find(item=>Number(item.chain)===scope.chain && item.genesis===scope.genesis);
    if (!owner || !registryCurrent || registry?.walletConnect?.relay!==connection.relay || !network) return false;
    try {
      validateConnectCheckpoint(connection.checkpoint,{board:owner.boardFingerprint,account:owner.address,
        generation:owner.generation,chain:Number(network.chain),genesis:network.genesis},Math.floor(Date.now()/1000));
      return true;
    } catch {return false;}
  });
  const selectedConnect=connectSelection===null ? undefined : savedConnections.find(item=>
    item.checkpoint.topic===(connectSelection ?? connections[0]?.checkpoint.topic));
  const nextConnectionExpiry=connections.length ? Math.min(registry!.expires,...connections.map(item=>item.checkpoint.expires)) : undefined;
  useEffect(()=>{
    if (screen!=='allowance' || nextConnectionExpiry===undefined) return;
    const timer=setTimeout(()=>refreshConnections(value=>value+1),Math.max(0,nextConnectionExpiry*1000-Date.now()));
    return ()=>clearTimeout(timer);
  },[screen,nextConnectionExpiry]);
  function openExternalConnection(connection?:StoredConnect,fromExplore=false) {
    exploreBack.current=fromExplore?'walletconnect':undefined;
    setConnectSelection(connection?.checkpoint.topic ?? null);
    const target=registry?.networks.find(network=>Number(network.chain)===connection?.checkpoint.scope.chain &&
      network.genesis===connection?.checkpoint.scope.genesis);
    if (target) setReceiveChain(target.chain);
    dappSigned.current=undefined;
    setScreen('walletconnect');
  }
  const openAllowance = (fromExplore=false) => {
    exploreBack.current=fromExplore?'allowance':undefined;
    setAllowanceConnection(undefined);setAllowanceRead(undefined);setTransfer(undefined);setTransferWire('');
    setAllowanceSpender('');
    if (registryCurrent && registry?.dapp?.allowance && visibleEvmNetworks.some(item=>item.chain===registry.dapp?.chain)) {
      setReceiveChain(registry.dapp.chain);
      setEvmAsset(registry.dapp.allowance.token);
      setAllowanceSpender(registry.dapp.allowance.spender);
    }
    setScreen('allowance');
  };
  function viewAllowanceConnections(origin:string,transport:'browser'|'walletconnect') {
    const owner=currentStorage.current?.verified;
    if (!owner || !chosenNetwork || registry?.dapp?.origin!==origin || registry.dapp.chain!==chosenNetwork.chain) return;
    openAllowance();
    setAllowanceConnection({origin,transport,chain:chosenNetwork.chain,account:owner.address});
    if (registry.dapp.allowance) {
      setEvmAsset(registry.dapp.allowance.token);
      setAllowanceSpender(registry.dapp.allowance.spender);
    }
  }

  function disconnectConnections(selected:StoredConnect[],all=false) {
    const owner=stored?.verified, epoch=allowanceEpoch.current;
    const topics=new Set(selected.map(item=>item.checkpoint.topic));
    if (!owner || !topics.size) return;
    work(async()=>{
      const removed:StoredConnect[]=[];
      await commit(head=>{
        if (!keepAllowanceResult(epoch) || head.verified?.boardFingerprint!==owner.boardFingerprint ||
            head.verified?.generation!==owner.generation || head.verified?.address!==owner.address) return head;
        const saved=storedWalletConnects(head);
        if (all && saved.some(item=>!topics.has(item.checkpoint.topic))) throw new Error('WC_SCOPE_CHANGED');
        removed.push(...saved.filter(item=>topics.has(item.checkpoint.topic)));
        return {...head,walletConnect:undefined,walletConnectSessions:saved.filter(item=>!topics.has(item.checkpoint.topic))};
      });
      if (!removed.length) return;
      const wc=registry?.walletConnect;
      if (!native || !wc) return;
      const sent=await Promise.allSettled(removed.map(item=>notifyStoredDisconnect(item,wc.relay,wc.projectId,
        async length=>fromBase64(await native.randomBytes(length)))));
      if (keepAllowanceResult(epoch)) setMessage(sent.some(item=>item.status==='rejected')
        ? t('이 기기의 연결 권한을 해제했습니다. 상대 연결은 만료될 때 종료됩니다.')
        : t('사이트 연결을 끊었습니다.'));
    },t('지갑 연결 해제를 저장하지 못했습니다.'));
  }

  function confirmDisconnectAll() {
    const epoch=allowanceEpoch.current, selected=[...savedConnections];
    Alert.alert(t('전체 연결 해제'),t('이 지갑의 모든 사이트 연결을 해제합니다. 온체인 토큰 사용 한도는 유지됩니다.'),[
      {text:t('취소'),style:'cancel'},
      {text:t('전체 연결 해제'),style:'destructive',onPress:()=>{
        if (keepAllowanceResult(epoch)) disconnectConnections(selected,true);
      }},
    ],{cancelable:true});
  }

  function readAllowance() {
    work(async () => {
      if (!native || !stored?.verified || !registryCurrent || !chosenNetwork || !chosenAsset || stored.pendingEvm ||
          chosenAsset.address==='0x'+'0'.repeat(40)) throw new Error('EVM_ALLOWANCE_SCOPE');
      const spender=normalizeEvmAddress(allowanceSpender.trim());
      const rpc=httpsEvmRpc(selectedRpc(chosenNetwork),(endpoint,packet)=>native.evmRpc(endpoint,packet));
      const epoch=allowanceEpoch.current;
      if (!keepAllowanceResult(epoch)) return;
      setAllowanceRead(undefined);
      const value=await finalizedAllowance(rpc,chosenNetwork.chain,chosenNetwork.genesis,chosenAsset.address,stored.verified.address,spender);
      if (keepAllowanceResult(epoch)) setAllowanceRead({scope:allowanceScope(chosenNetwork.chain,chosenNetwork.genesis,chosenAsset.address,spender),value});
    }, networkFailure('확정된 토큰 사용 한도를 조회하지 못했습니다.'));
  }

  async function prepareDappLogin(message:string, origin:string) {
    const entry=stored?.evmRegistry, owner=stored?.verified;
    if (!native || !entry?.anchor || !owner || !registryCurrent || !registry?.dapp ||
        registry.dapp.origin!==origin || registry.dapp.chain!==chosenNetwork?.chain) throw new Error('DAPP_SCOPE');
    const scope={board:owner.boardFingerprint,account:owner.address,generation:owner.generation,
      chain:Number(chosenNetwork.chain),genesis:chosenNetwork.genesis};
    const service=httpsEvmRpc(entry.endpoint,(endpoint,packet)=>native.evmRpc(endpoint,packet));
    const issued=await service('nu54_walletLogin',[{token:entry.token,message,origin}]);
    const review=readIssuedLogin(issued,message,origin,scope,BigInt(registry.leaseRevision),
      hexToBytes(entry.anchor),Math.floor(Date.now()/1000));
    return {issued,nonce:review.nonce,expires:review.expires};
  }

  async function signDappLogin(handle:string,issued:unknown,message:string,origin:string,signal:AbortSignal) {
    if (!native || !profile || !stored?.verified || !chosenNetwork || !registryCurrent || signal.aborted)
      throw new Error('DAPP_SESSION');
    const owner=stored.verified;
    const session=new OwnerSession(Ble,native,profile);
    connection.current=session;
    const cancel=()=>session.close();
    signal.addEventListener('abort',cancel,{once:true});
    try {
      await session.connect(handle);
      if (signal.aborted) throw new Error('DAPP_SESSION');
      return await session.signDappLogin(issued,message,origin,{board:owner.boardFingerprint,account:owner.address,
        generation:owner.generation,chain:Number(chosenNetwork.chain),genesis:chosenNetwork.genesis});
    } finally {
      signal.removeEventListener('abort',cancel);
      session.close();
      if (connection.current===session) connection.current=undefined;
    }
  }

  async function prepareDappSpend(recipient:string,amount:bigint,external?:ExternalSpendContext) {
    const entry=stored?.evmRegistry, owner=stored?.verified;
    const asset='0x'+'0'.repeat(40);
    if (!native || !entry?.anchor || !owner || stored.pendingEvm || !registryCurrent || !registry || !chosenNetwork ||
        !chosenNetwork.assets.some(item=>item.address===asset)) throw new Error('DAPP_SPEND_PROFILE');
    const service=httpsEvmRpc(entry.endpoint,(endpoint,packet)=>native.evmRpc(endpoint,packet));
    const wire=await service(external ? 'nu54_walletExternalSpend' : 'nu54_walletQuote',
      [{token:entry.token,chain:chosenNetwork.chain,asset,recipient,amount:amount.toString(),...external}]);
    if (typeof wire!=='string') throw new Error('EVM_TRANSFER_WIRE');
    const draft=readDirectTransfer(wire,BigInt(Math.floor(Date.now()/1000)));
    verifyTransferAuthority(draft,hexToBytes(entry.anchor),owner.boardFingerprint,BigInt(owner.generation),owner.address,BigInt(registry.leaseRevision));
    const actual=draft.externalContext;
    if ((!external!==!actual) || external && (external.origin!==actual?.origin || external.pairingTopic!==actual.pairingTopic ||
        external.sessionTopic!==actual.sessionTopic || external.peerKey!==actual.peerKey)) throw new Error('EVM_WC_SCOPE');
    if (draft.revoke || draft.allowanceContext || draft.chain!==chosenNetwork.chain || draft.genesis!==chosenNetwork.genesis ||
        draft.asset!==asset || draft.recipient!==recipient || draft.amount!==amount || draft.maximum>BigInt(chosenNetwork.feeLimit))
      throw new Error('DAPP_SPEND_MISMATCH');
    return {wire,fee:draft.maximum};
  }

  const dappAllowancePolicy=registry?.dapp?.chain===chosenNetwork?.chain ? registry?.dapp?.allowance : undefined;
  async function prepareDappAllowance(token:string,spender:string,amount:bigint,origin:string,
      sessionTopic:string,sessionExpires:number,external?:ExternalSpendContext) {
    const entry=stored?.evmRegistry, owner=stored?.verified, allowance=dappAllowancePolicy;
    const asset=chosenNetwork?.assets.find(item=>item.address===token);
    if (!native || !entry?.anchor || !owner || stored.pendingEvm || !registryCurrent || !registry || !chosenNetwork ||
        !asset || !allowance || origin!==registry.dapp?.origin || token!==allowance.token || spender!==allowance.spender)
      throw new Error('DAPP_ALLOWANCE_PROFILE');
    const service=httpsEvmRpc(entry.endpoint,(endpoint,packet)=>native.evmRpc(endpoint,packet));
    const wire=await service('nu54_walletDappAllowance',[{token:entry.token,chain:chosenNetwork.chain,asset:token,
      recipient:spender,amount:amount.toString(),origin,sessionTopic,sessionExpires,
      transport:external ? 'walletconnect' : 'browser',
      ...(external && {pairingTopic:external.pairingTopic,peerKey:external.peerKey})}]);
    if (typeof wire!=='string') throw new Error('EVM_TRANSFER_WIRE');
    const draft=readDirectTransfer(wire,BigInt(Math.floor(Date.now()/1000)));
    verifyTransferAuthority(draft,hexToBytes(entry.anchor),owner.boardFingerprint,BigInt(owner.generation),owner.address,BigInt(registry.leaseRevision));
    const actual=draft.allowanceContext;
    if (!actual || origin!==actual.origin || sessionTopic!==actual.sessionTopic ||
        external?.pairingTopic!==actual.external?.pairingTopic || external?.peerKey!==actual.external?.peerKey ||
        draft.expires>BigInt(sessionExpires) || draft.chain!==chosenNetwork.chain || draft.genesis!==chosenNetwork.genesis ||
        draft.asset!==token || draft.recipient!==spender || draft.amount!==amount || actual.decimals!==asset.decimals ||
        actual.maximum!==BigInt(allowance.maximum) || draft.feeLimit!==BigInt(chosenNetwork.feeLimit))
      throw new Error('DAPP_ALLOWANCE_MISMATCH');
    return {wire,fee:draft.maximum,amountLabel:formatEvmTokenAmount(amount,asset)};
  }

  async function signDappSpend(handle:string,wire:string,signal:AbortSignal) {
    if (!native || !profile || !stored?.verified || !chosenNetwork || !registryCurrent || stored.pendingEvm ||
        dappSigned.current || signal.aborted) throw new Error('DAPP_SPEND_SESSION');
    const owner=stored.verified;
    const draft=readDirectTransfer(wire,BigInt(Math.floor(Date.now()/1000)));
    const ensureCurrent=()=>{
      const latest=currentStorage.current;
      if (signal.aborted || (draft.allowanceContext && (
          draft.expires<=BigInt(Math.floor(Date.now()/1000)) ||
          latest?.verified?.boardFingerprint!==owner.boardFingerprint || latest.verified.generation!==owner.generation ||
          latest.verified.address!==owner.address || latest.evmRegistry?.payload!==stored.evmRegistry?.payload ||
          !builtInEvmEnabled(draft.chain,latest.walletRead?.disabled)))) throw new Error('DAPP_SPEND_SESSION');
    };
    ensureCurrent();
    const rpc=httpsEvmRpc(selectedRpc(chosenNetwork),(endpoint,packet)=>native.evmRpc(endpoint,packet));
    await checkEvmNetwork(rpc,draft.chain,draft.genesis);
    ensureCurrent();
    const session=new OwnerSession(Ble,native,profile);
    connection.current=session;
    const cancel=()=>session.close();
    signal.addEventListener('abort',cancel,{once:true});
    try {
      await session.connect(handle);
      ensureCurrent();
      const request=await session.newSigningRequestId();
      const context={request,unsigned:bytesToHex(draft.raw,true),chain:draft.chain,genesis:draft.genesis,signer:owner.address};
      const committed = await persistWalletSignature(stored, 'evm', wire, Number(draft.expires), commit, async () => {
        ensureCurrent();
        const checked=await session.signDirectTransfer(wire,owner.address,BigInt(owner.generation),
          ()=>waitOnBoard({flow:'sign',step:'display'},t('독립 화면에서 체인, 수취인, 금액과 수수료를 확인하고 보드에서 승인하세요.')),request);
        return {pendingEvm:{...checked,chain:draft.chain,sender:owner.address,expires:Number(draft.expires),
          submitted:false,genesis:draft.genesis,profileRevision:Number(draft.profileRevision),leaseRevision:draft.leaseRevision.toString(),rpc:selectedRpc(chosenNetwork)}};
      },context);
      const pending = committed.pendingEvm!;
      dappSigned.current=pending;
      return pending.hash;
    } finally {
      // This runs outside work(), so the board step it opened must be closed here.
      setBoardWait(undefined);
      signal.removeEventListener('abort',cancel);
      session.close();
      if (connection.current===session) connection.current=undefined;
    }
  }

  async function submitDappSpend(hash:string,signal:AbortSignal) {
    const pending=dappSigned.current;
    if (!native || !stored || !pending?.rpc || !pending.genesis || pending.hash!==hash ||
        pending.submitted || pending.expires<=Date.now()/1000 || signal.aborted) throw new Error('DAPP_SPEND_SESSION');
    const rpc=httpsEvmRpc(pending.rpc,(endpoint,packet)=>native.evmRpc(endpoint,packet));
    await checkEvmNetwork(rpc,pending.chain,pending.genesis);
    if (signal.aborted || pending.expires<=Date.now()/1000) throw new Error('DAPP_SPEND_SESSION');
    const current = currentStorage.current;
    if (!current || current.pendingEvm?.hash !== hash || current.pendingEvm.submitted || !builtInEvmEnabled(pending.chain, current.walletRead?.disabled)) {
      throw new Error('DAPP_SPEND_SESSION');
    }
    await save({...current,pendingEvm:{...pending,submitted:true}}, current);
    dappSigned.current=undefined;
    if (signal.aborted || Date.now()/1000>=pending.expires) throw new Error('DAPP_SPEND_SESSION');
    if (await rpc('eth_sendRawTransaction',[pending.raw])!==hash) throw new Error('EVM_RPC_WRONG_HASH');
  }

  async function verifyDappBoard(handle:string) {
    if (!native || !profile || !stored?.verified) throw new Error('DAPP_ACCOUNT');
    const session=new OwnerSession(Ble,native,profile);
    try {
      await session.connect(handle);
      await session.verifiedEvmAddress(stored.verified.address,BigInt(stored.verified.generation));
      const authority=await session.evmAuthority();
      const owner=stored.verified;
      if (bytesToHex(authority.key,false)!==stored.evmRegistry?.anchor) throw new Error('DAPP_AUTHORITY');
      authenticateRegistry(stored.evmRegistry,authority.key,authority.revision,
        {device:owner.boardFingerprint,address:owner.address,generation:owner.generation},Math.floor(Date.now()/1000));
    } finally {session.close();}
  }

  const externalBoard=useRef('');
  const serviceExternalRequest:SessionOptions['request']=async(action,proposal,signal,sessionTopic,expiry)=>{
    const handle=externalBoard.current;
    const checkLive=()=>{
      if (signal.aborted || Math.floor(Date.now()/1000)>=expiry) throw new Error('WC_CLOSED');
    };
    checkLive();
    if (!handle || !proposal.verified || !registryCurrent || !chosenNetwork ||
        registry?.dapp?.origin!==proposal.origin || registry.dapp.chain!==chosenNetwork.chain)
      throw new Error('WC_UNVERIFIED_PEER');
    if (action.kind==='login') {
      const proof=await prepareDappLogin(action.message,proposal.origin);
      const decision=await reviewAllowancePrompt('외부 DApp 로그인',proposal.origin+'\n'+proposal.account+
        '\n'+evmProfiles[chosenNetwork.chain].label+' · '+proposal.chain+'\nNonce '+proof.nonce+
        '\n기한 '+new Date(proof.expires*1000).toISOString()+'\n\n'+action.message+
        '\n\n독립 화면의 사이트와 보드 요청을 확인하세요. 온체인 거래는 제출하지 않습니다.',signal,'세션 보기');
      if (decision==='connections') return viewAllowanceConnection;
      if (decision!=='approve') throw new Error('WC_REFUSED');
      checkLive();
      if (Math.floor(Date.now()/1000)>=proof.expires) throw new Error('WC_CLOSED');
      const signature=await signDappLogin(handle,proof.issued,action.message,proposal.origin,signal);
      checkLive();
      if (Math.floor(Date.now()/1000)>=proof.expires) throw new Error('WC_CLOSED');
      return {signature,nonce:proof.nonce};
    }
    const {origin,pairingTopic,peerKey}=proposal;
    const external={origin,pairingTopic,sessionTopic,peerKey};
    let quote:{wire:string;fee:bigint};
    if (action.kind==='allowance') {
      const approval=await prepareDappAllowance(action.token,action.spender,action.amount,origin,sessionTopic,expiry,external);
      quote=approval;
      const decision=await reviewAllowancePrompt('외부 DApp 토큰 권한 부여',origin+'\n'+evmProfiles[chosenNetwork.chain].label+
          ' · '+proposal.chain+'\nToken '+action.token+'\nSpender '+action.spender+'\nAllowance '+approval.amountLabel+
          '\n최대 수수료 '+quote.fee+' wei\n독립 화면의 토큰과 대상을 대조한 뒤 보드에서 확인하세요.',signal);
      if (decision==='connections') return viewAllowanceConnection;
      if (decision!=='approve') throw new Error('WC_REFUSED');
    } else if (action.kind==='spend') {
      quote=await prepareDappSpend(action.recipient,action.value,external);
      if (!await reviewPrompt('외부 DApp 송금',origin+'\n'+proposal.chain+'\n'+action.recipient+
          '\n'+action.value+' wei\n최대 수수료 '+quote.fee+' wei',signal)) throw new Error('WC_REFUSED');
    } else throw new Error('WC_METHOD');
    checkLive();
    const hash=await signDappSpend(handle,quote.wire,signal);
    if (!await reviewPrompt('서명한 거래 전송',proposal.chain+'\n'+hash,signal)) throw new Error('WC_REFUSED');
    checkLive();
    await submitDappSpend(hash,signal);
    return hash;
  };

  function enrollEvmRegistry(handle:string) {
    work(async () => {
      if (!native || !profile || !stored?.verified || !/^[a-f0-9]{64}$/.test(registryToken.trim())) throw new Error('EVM_ACCOUNT_BINDING');
      const endpoint=registryEndpoint.trim(), token=registryToken.trim();
      const exchange=httpsEvmRpc(endpoint,(url,packet)=>native.evmRpc(url,packet));
      const response=await exchange('nu54_walletRegistry',[{token}]);
      if (!response || typeof response!=='object') throw new Error('EVM_REGISTRY');
      const session=new OwnerSession(Ble,native,profile);
      connection.current=session;
      try {
        await session.connect(handle);
        await session.verifiedEvmAddress(stored.verified.address,BigInt(stored.verified.generation));
        const trust=await session.evmAuthority();
        const {entry,registry:verified}=authenticateRegistry({...response,endpoint,token},trust.key,trust.revision,
          {device:profile.device,address:stored.verified.address,generation:stored.verified.generation},Math.floor(Date.now()/1000));
        await save({...stored,evmRegistry:{...entry,anchor:bytesToHex(trust.key,false)}});
        setReceiveChain(verified.networks[0].chain);
        setRegistryToken('');
        setTransfer(undefined);
        setMessage(t('지갑 네트워크 설정을 확인했습니다.'));
      } finally {
        session.close();
        connection.current=undefined;
      }
    },t('이 보드에서 서명된 네트워크 설정을 확인할 수 없습니다.'));
  }

  function confirmEvmReceive(handle:string) {
    work(async () => {
      setEvmReceiveBalance(undefined);
      if (!native || !profile || !stored?.verified || !chosenNetwork || !registryCurrent ||
          stored.verified.boardFingerprint !== profile.device) throw new Error('EVM_ACCOUNT_BINDING');
      const rpc = httpsEvmRpc(selectedRpc(chosenNetwork),(endpoint,packet)=>native.evmRpc(endpoint,packet));
      await checkEvmNetwork(rpc,chosenNetwork.chain,chosenNetwork.genesis);
      const session = new OwnerSession(Ble,native,profile);
      connection.current = session;
      try {
        await session.connect(handle);
        setReceiveAddress(await session.verifiedEvmAddress(stored.verified.address,BigInt(stored.verified.generation)));
      } finally {
        session.close();
        connection.current = undefined;
      }
    },networkFailure('네트워크 또는 보드 수신 주소를 확인하지 못했습니다.'));
  }

  function quoteEvmTransfer(revoke=false) {
    work(async () => {
      const entry=stored?.evmRegistry;
      if (!native || !entry?.anchor || !stored?.verified || !registry || !chosenNetwork || !chosenAsset || !registryCurrent)
        throw new Error('EVM_REGISTRY');
      const recipient=normalizeEvmAddress((revoke ? allowanceSpender : evmRecipient).trim());
      const amount=revoke ? 0n : parseHumanAmount(evmAmount.trim(),chosenAsset.decimals);
      const authority=httpsEvmRpc(entry.endpoint,(endpoint,packet)=>native.evmRpc(endpoint,packet));
      const epoch=allowanceEpoch.current;
      if (revoke) {
        if (!keepAllowanceResult(epoch)) return;
        const rpc=httpsEvmRpc(selectedRpc(chosenNetwork),(endpoint,packet)=>native.evmRpc(endpoint,packet));
        const value=await finalizedAllowance(rpc,chosenNetwork.chain,chosenNetwork.genesis,chosenAsset.address,stored.verified.address,recipient);
        if (!keepAllowanceResult(epoch)) return;
        setAllowanceRead({scope:allowanceScope(chosenNetwork.chain,chosenNetwork.genesis,chosenAsset.address,recipient),value});
        if (value===0n) throw new Error('EVM_ALLOWANCE_ALREADY_ZERO');
      }
      const wire=await authority(revoke ? 'nu54_walletRevokeAllowance' : 'nu54_walletQuote',
        [{token:entry.token,chain:chosenNetwork.chain,asset:chosenAsset.address,recipient,...(revoke ? {} : {amount:amount.toString()})}]);
      if (revoke && !keepAllowanceResult(epoch)) return;
      if (typeof wire!=='string') throw new Error('EVM_TRANSFER_WIRE');
      const reviewed=readDirectTransfer(wire,BigInt(Math.floor(Date.now()/1000)));
      if (reviewed.revoke!==revoke || reviewed.allowanceContext || reviewed.externalContext || reviewed.chain!==chosenNetwork.chain || reviewed.genesis!==chosenNetwork.genesis || reviewed.asset!==chosenAsset.address ||
          reviewed.recipient!==recipient || reviewed.amount!==amount || reviewed.maximum>BigInt(chosenNetwork.feeLimit))
        throw new Error('EVM_TRANSFER_MISMATCH');
      verifyTransferAuthority(reviewed,hexToBytes(entry.anchor),registry.device,BigInt(registry.generation),registry.sender,BigInt(registry.leaseRevision));
      setTransferWire(wire);
      setTransfer(reviewed);
    },t('전송 정책 또는 네트워크를 확인하세요.'));
  }

  function signTransfer(handle:string) {
    work(async () => {
      if (!native || !profile || !stored?.verified || !transfer || !chosenNetwork || !registryCurrent || stored.pendingEvm ||
          stored.verified.boardFingerprint !== profile.device) throw new Error('EVM_ACCOUNT_BINDING');
      const rpc = httpsEvmRpc(selectedRpc(chosenNetwork),(endpoint,packet)=>native.evmRpc(endpoint,packet));
      const epoch=allowanceEpoch.current;
      if (transfer.revoke) {
        if (!keepAllowanceResult(epoch)) return;
        if (screen!=='allowance' || !allowance || !allowance.value ||
            allowance.scope!==allowanceScope(transfer.chain,transfer.genesis,transfer.asset,transfer.recipient))
          throw new Error('EVM_ALLOWANCE_SCOPE');
        const value=await finalizedAllowance(rpc,transfer.chain,transfer.genesis,transfer.asset,stored.verified.address,transfer.recipient);
        if (!keepAllowanceResult(epoch)) return;
        if (value!==allowance.value) {
          setAllowanceRead({scope:allowance.scope,value});
          setTransfer(undefined);
          setTransferWire('');
          throw new Error('EVM_ALLOWANCE_CHANGED');
        }
      } else await checkEvmNetwork(rpc,transfer.chain,transfer.genesis);
      const session = new OwnerSession(Ble,native,profile);
      connection.current = session;
      try {
        await session.connect(handle);
        if (transfer.revoke && !keepAllowanceResult(epoch)) return;
        const owner = stored.verified;
        const wire = transferWire.trim();
        const request=await session.newSigningRequestId();
        if (transfer.revoke && !keepAllowanceResult(epoch)) return;
        const context={request,unsigned:bytesToHex(transfer.raw,true),chain:transfer.chain,genesis:transfer.genesis,signer:owner.address};
        await persistWalletSignature(stored, 'evm', wire, Number(transfer.expires), commit, async () => {
          if (transfer.revoke && !keepAllowanceResult(epoch)) throw new Error('EVM_ALLOWANCE_INACTIVE');
          const checked = await session.signDirectTransfer(wire, owner.address, BigInt(owner.generation),
            () => waitOnBoard({flow:'sign',step:'display'},t(transfer.revoke ? '독립 화면에서 토큰, 권한 대상, 권한 0과 수수료를 확인하세요.' : '독립 화면에서 체인, 수취인, 금액과 수수료를 확인하고 보드에서 승인하세요.')),request);
          return {pendingEvm:{...checked,chain:transfer.chain,sender:owner.address,
            expires:Number(transfer.expires),submitted:false,genesis:transfer.genesis,
            profileRevision:Number(transfer.profileRevision),leaseRevision:transfer.leaseRevision.toString(),rpc:selectedRpc(chosenNetwork)}};
        },context);
        setTransfer(undefined);
        setTransferWire('');
        setMessage(t('서명을 저장했습니다. 지정한 네트워크에서 전송하거나 상태를 확인하세요.'));
      } finally {
        session.close();
        connection.current = undefined;
      }
    },error=>t(error instanceof Error && error.message==='EVM_ALLOWANCE_CHANGED'
      ? '확정 한도가 변경되었습니다. 다시 검토하세요.' : '보드에서 정확한 전송 서명을 확인하지 못했습니다.'));
  }

  function currentTransfer(action:'submit'|'query') {
    const pending = stored?.pendingEvm;
    if (!stored || !pending) return;
    work(async () => {
      if (!native) throw new Error('NATIVE_PROFILE_REQUIRED');
      if (screen==='allowance' && (!pendingAllowance || pending.sender!==stored.verified?.address))
        throw new Error('EVM_ALLOWANCE_SCOPE');
      const rpc = httpsEvmRpc(rpcOverride[rpcKey(pending.chain, pending.genesis ?? '')] ?? pending.rpc ?? rpcEndpoint.trim(),(endpoint,packet)=>native.evmRpc(endpoint,packet));
      const genesis = pending.genesis;
      if (!genesis) throw new Error('EVM_UNBOUND_LEGACY_TRANSFER');
      verifyStoredEvmWire(pending.raw,pending.hash,pending.chain);
      const epoch=allowanceEpoch.current;
      const change=storedAllowanceChange(pending.raw);
      const positive=change && change.amount>0n;
      if (change && !keepAllowanceResult(epoch)) return;
      if (action === 'submit') {
        if (positive) throw new Error('DAPP_ALLOWANCE_SESSION');
        if (!builtInEvmEnabled(pending.chain, stored.walletRead?.disabled)) throw new Error('EVM_NETWORK_DISABLED');
        if (pending.submitted || Date.now()/1000 >= pending.expires) throw new Error('EVM_ALREADY_ATTEMPTED');
        await checkEvmNetwork(rpc,pending.chain,genesis);
        if (Date.now()/1000 >= pending.expires) throw new Error('EVM_TRANSFER_EXPIRED');
        if (change && !keepAllowanceResult(epoch)) return;
        await save({...stored,pendingEvm:{...pending,submitted:true}});
        if (change && !keepAllowanceResult(epoch)) return;
        if (await rpc('eth_sendRawTransaction',[pending.raw]) !== pending.hash) throw new Error('EVM_RPC_WRONG_HASH');
        setMessage(t('전송을 요청했습니다. 승인된 해시로 상태를 다시 확인하세요.'));
        return;
      }
      if (change) setAllowanceRead(previous=>previous?.activeHash===pending.hash ? undefined : previous);
      let state;
      if (change) {
        const result=await queryEvmAllowanceChange(rpc,pending.chain,genesis,pending.hash,change.token,pending.sender,change.spender);
        if (!keepAllowanceResult(epoch)) return;
        state=result.state;
        const scope=allowanceScope(pending.chain,genesis,change.token,change.spender);
        setAllowanceRead({scope,value:result.value,activeHash:pending.hash,result:state});
        if (state==='reverted') return;
      } else {
        state=await queryEvmTransfer(rpc,pending.chain,genesis,pending.hash);
      }
      // Phone RPC observations do not settle the independent board reservation.
      // Preserve signed and unsigned originals across restarts until board evidence matches.
      if(state==='finalized' || state==='reverted')setMessage(language==='ko' ?
        '선택한 RPC가 확정 결과를 보고했습니다. 보드 정산 전까지 서명 원문과 미결을 보관합니다.' :
        'The selected RPC reported a final result. The signed original and pending record remain until board reconciliation.');
      else setMessage(t(state==='included' ? '블록에 포함되었습니다. 최종 확정을 기다리세요.' : '아직 선택한 네트워크에서 확인되지 않습니다.'));
    },networkFailure('결과가 불확실합니다. 같은 해시로 네트워크 상태를 다시 확인하세요.'));
  }

  function reclaimReceipt(handle:string) {
    work(async () => {
      if (!native || !profile) throw new Error('PROFILE_REQUIRED');
      const session = new OwnerSession(Ble,native,profile);
      connection.current = session;
      try {
        await session.connect(handle);
        const result = await session.reclaimPaymentReceipt(reclaimWire.trim());
        setReclaimWire('');
        setMessage(t(result === 'reclaimed' ? '확정된 지급 기록의 보드 공간을 회수했습니다.' : '해당 지급 기록은 보드에 없습니다.'));
      } finally {
        session.close();
        connection.current = undefined;
      }
    },t('보드에서 확정된 지급 기록을 회수하지 못했습니다.'));
  }

  function endRecovery() {
    connection.current?.close();
    connection.current = undefined;
    setRestore(undefined);
    setRecoveryTarget(undefined);
  }

  function endBackup() {
    connection.current?.close();
    connection.current = undefined;
    setBackup(undefined);
    setBackupBlocked(false);
  }

  function startBackup(handle: string) {
    work(async () => {
      if (!native || !profile || !stored) throw new Error('PROFILE_REQUIRED');
      endBackup();
      const session = new OwnerSession(Ble,native,profile);
      connection.current = session;
      try {
        await session.connect(handle);
        const opened = await session.beginBackup();
        await save({...stored,backupCheckpoint:opened.checkpoint});
        setBackup(opened);
        setMessage('기기 표시등이 대기 중이면 SW3을 길게 누르고 기기에서만 PIN을 입력하세요. 독립 복구 장치 화면에서 요청 번호를 대조하고 승인하세요.');
      } catch (error) {
        endBackup();
        if (!stored.verified && error instanceof Error && error.message === 'BACKUP_UNAVAILABLE') {
          setBackupBlocked(true);
          return;
        }
        throw error;
      }
    }, error => error instanceof Error && error.message === 'BACKUP_UNAVAILABLE'
      ? '이 기기에서 독립 백업 요청을 열 수 없습니다. 등록된 원본 기기와 별도 복구 장치를 확인하세요.'
      : '원본 기기에서 백업 요청을 확인하지 못했습니다. 다시 연결하세요.');
  }

  function checkBackup() {
    work(async () => {
      const session = connection.current;
      if (!session) throw new Error('BACKUP_SESSION_REQUIRED');
      const result = await session.checkBackup();
      if (!result.verified) {
        setMessage('독립 복구 확인을 기다리고 있습니다. 백업 후 별도 기기에서 복원 시험을 진행하고 원본 기기의 확인을 마치세요.');
        return;
      }
      await saveBackedUpAddress(session, result.status, result.address);
      setStatus(result.status);
      endBackup();
      setScreen('owner');
      setMessage('원본 기기가 독립 복원 시험 완료를 확인했습니다.');
    }, '백업 확인 상태를 읽을 수 없습니다. 다시 연결해 상태를 확인하세요.');
  }

  function verifyRecoveryTarget(handle:string) {
    work(async()=>{
      if(!native||!recoveryBoard||!stored)throw new Error('PROFILE_REQUIRED');
      endRecovery();
      const independentPins=pins.map(value=>value.trim());
      const trust=importRecoveryTrust(trustDoc,independentPins,recoveryBoard.device);
      if(independentPins[1]!==recoveryBoard.boardSPKI || stored.verified&&
          ![trust.source,recoveryBoard.device].includes(stored.verified.boardFingerprint))throw new Error('RECOVERY_REGISTRATION');
      const session=new OwnerSession(Ble,native,recoveryBoard);
      connection.current=session;
      try{
        await session.connect(handle);
        const target=await session.maintenanceStatus();
        if(target.state!=='active'||target.device!==recoveryBoard.device||connection.current!==session)throw new Error('RECOVERY_TARGET');
        setRecoveryTarget(target);
      }catch(error){
        if(connection.current===session)endRecovery();
        throw error;
      }
    },error=>error instanceof Error&&['RECOVERY_REGISTRATION','RECOVERY_EVIDENCE_REJECTED'].includes(error.message)
      ? '복구 등록서와 독립 지문이 원래 등록 대상과 일치하는지 확인하세요.'
      : '선택한 후보가 등록된 복구 대상인지 확인하지 못했습니다. 복구 요청을 시작하지 않았습니다.');
  }

  function startRecovery() {
    work(async()=>{
      const session=connection.current;
      if(!session||!recoveryTarget)throw new Error('RECOVERY_TARGET');
      try{
        const context=await session.beginRecovery(recoveryTarget);
        if(connection.current!==session)return;
        setRestore(context);
        setMessage('별도로 등록한 복구 장치에서 기기와 요청 번호를 확인한 뒤 복구를 진행하세요.');
      }catch(error){
        if(connection.current===session)endRecovery();
        throw error;
      }
    },error=>error instanceof Error&&error.message==='RECOVERY_UNAVAILABLE'
      ? '이 기기는 독립 복구 요청을 제공하지 않습니다. 지원되는 빈 기기를 사용하세요.'
      : '새 기기의 인증된 복구 요청을 열 수 없습니다. 지원 프로필과 검증자 등록 정보를 확인하세요.');
  }

  function checkRecovery() {
    work(async () => {
      const session = connection.current;
      if (!session || !stored || !recoveryBoard) throw new Error('RECOVERY_SESSION_REQUIRED');
      const trust = importRecoveryTrust(trustDoc,pins.map(value => value.trim()),recoveryBoard.device);
      const verified = await session.checkRecovery(trust);
      if (!verified) {
        setMessage('기기에서 아직 복구 증거를 받지 못했습니다.');
        return;
      }
      const previous = stored.verified;
      if (previous && (previous.address !== verified.address || previous.network !== verified.network ||
          (previous.boardFingerprint === verified.source && previous.generation !== verified.sourceGeneration) ||
          ![verified.source,verified.target].includes(previous.boardFingerprint))) throw new Error('ACCOUNT_CHANGED');
      const current = await session.status();
      const own = await session.identity();
      if (current.state !== 'active' || current.generation !== BigInt(verified.generation) ||
          current.owner !== own.fingerprint || current.root !== 2 || current.profile !== 1 || !current.backedUp ||
          await session.backedUpAddress(current) !== verified.address) throw new Error('OWNER_MISMATCH');
      await save({...stored,mode:'personal',enrollment:recoveryBoard,backupCheckpoint:undefined,evmRegistry:undefined,verified:{
        boardFingerprint:verified.target,address:verified.address,network:verified.network,generation:verified.generation,
      }});
      setStatus(current);
      endRecovery();
      setProfile(recoveryBoard);
      setRecoveryOrigin(undefined);
      setScreen('owner');
      setMessage('복구된 주소와 계정을 확인했습니다. 원래 기기를 다시 사용할 수 있다면 승인 권한을 별도로 철회하고 보관 상태를 확인하세요.');
    }, '복구 결과를 확인할 수 없습니다. 요청이 만료되었으면 다시 시작하세요.');
  }

  async function read(text: string) {
    if (!native) throw new Error('TTS_UNAVAILABLE');
    await native.speak(t(text), language);
  }

  if (needsHomeProtection && !homeProtected) {
    return <ScrollView contentContainerStyle={styles.content}>
      <Text style={styles.body}>{protectionError ? (language === 'ko' ? '지갑 화면 보호를 시작하지 못했습니다.' : 'Wallet screen protection could not start.') : (language === 'ko' ? '지갑 화면 보호를 준비하고 있습니다.' : 'Preparing wallet screen protection.')}</Text>
      {protectionError && <Action title={language === 'ko' ? '다시 시도' : 'Try again'} run={retryProtection} />}
    </ScrollView>;
  }

  if(screen==='travel-privacy'){
    return <TravelPrivate back={()=>setScreen('settings')} backLabel={language==='ko'?'설정으로 돌아가기':'Return to Settings'} places={[]} />;
  }
  if (screen==='recovery-cloud' && cloudRecoveryAvailable) {
    return <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled"
      automaticallyAdjustKeyboardInsets keyboardDismissMode="on-drag">
      <CloudWalletScreen profile={cloudProfile} recoveryLanguage={language}
        back={()=>setScreen('recovery-cloud-info')} backLabel={language==='ko'?'복구 안내로 돌아가기':'Back to recovery information'} />
    </ScrollView>;
  }
  if(screen==='cloud'&&cloudProfile){
    return <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled"
      automaticallyAdjustKeyboardInsets keyboardDismissMode="on-drag">
      <CloudWalletScreen profile={cloudProfile} back={()=>setScreen('profiles')}/>
    </ScrollView>;
  }
  if(screen==='credentials'){
    return <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled"
      automaticallyAdjustKeyboardInsets keyboardDismissMode="on-drag">
      <Text accessibilityRole="header" style={styles.title}>{language==='ko'?'내 인증수단':'My sign-in methods'}</Text>
      <Text style={styles.body}>{language==='ko'?'개인 보드는 소셜 가입 없이 사용할 수 있습니다.':'The personal board works without a social sign-in.'}</Text>
      {cloudProfile ? <CloudWalletScreen profile={cloudProfile} back={()=>setScreen('settings')} backLabel={language==='ko'?'설정으로 돌아가기':'Return to Settings'} />
        : <><Text style={styles.body}>{language==='ko'?'이 배포에서 클라우드 계정 연결을 사용할 수 없습니다.':'Cloud account connection is unavailable in this deployment.'}</Text>
          <Action title={language==='ko'?'설정으로 돌아가기':'Return to Settings'} run={()=>setScreen('settings')}/></>}
    </ScrollView>;
  }
  const isPersonal = stored?.mode === 'personal';
  const isOwner = screen === 'owner';
  const settingsScreen=screen==='settings'||screen==='settings-access'||screen==='settings-diagnostics'||screen==='policy'||screen==='policy-applied';
  const maintenanceScreen=screen==='firmware'||screen==='settings-device'||screen==='policy-device';
  const verifiedWallet=stored?.verified;
  const boardStatusVerified=!!verifiedWallet && (
    (status?.state==='active' && status.device===verifiedWallet.boardFingerprint && status.generation===BigInt(verifiedWallet.generation)) ||
    (policyView?.board===verifiedWallet.boardFingerprint && policyView.generation===BigInt(verifiedWallet.generation)) ||
    (verifiedIdleLock?.board===verifiedWallet.boardFingerprint && verifiedIdleLock.generation===verifiedWallet.generation));

  const keyboardForm=screen==='walletconnect'||screen==='recovery'||screen==='replace-phone'||settingsScreen;
  const toggleBalance = () => {
    if (!stored) return;
    const walletRead = stored.walletRead ?? defaultReadState;
    work(() => save({...stored, walletRead:{...walletRead, selection:{...walletRead.selection, hidden:!walletRead.selection.hidden}}}),
      language === 'ko' ? '잔액 설정을 저장할 수 없습니다.' : 'Could not save balance visibility.');
  };
  const openLife = (next: BoardLifeScreen) => {setMessage(''); setFound([]); setScreen(next);};
  const boardRegistered = status?.state === 'active' && status.device === profile?.device;
  const boardAccount = stored ? stored.verified ?? null : undefined;
  const connectInstalledBoard = () => {setFromInstall(false); setScreen('owner');};
  const rejectAccessibilityPractice = () => {
    if (!stored || !native) return;
    work(async () => {
      await save({...stored,practice:practiceDecision(beginPractice('matching'),'reject'),exercised:true});
      setPracticeWarning(false);
      setScreen('practice');
    }, language==='ko' ? '연습 기록을 저장할 수 없습니다.' : 'Could not save the practice result.');
  };
  const practiceOrder = practiceOrderText(language, stored?.practice?.scenario);
  const routeEnrollmentForm = recoveryOrigin && !recoveryBoard && (screen==='replace-phone' || screen==='recovery') && <>
    <Text style={styles.body}>{screen==='replace-phone'
      ? (language==='ko'?'원래 보드의 발급 등록서와 독립 지문을 입력하세요.':'Enter the original board enrollment and its independently checked fingerprints.')
      : (language==='ko'?'복구 대상인 새 보드의 발급 등록서와 독립 지문을 입력하세요.':'Enter the replacement board enrollment and its independently checked fingerprints.')}</Text>
    {([
      ['document','발급 등록서',8000],
      ['device','기기 식별값',64],
      ['fingerprint','발급 문서의 서버 공개키 지문',64],
    ] as const).map(([field,label,maxLength]) => <TextInput placeholderTextColor={color.mute} key={field} testID={'w06-board-enrollment-'+field}
      accessibilityLabel={t(label)} placeholder={t(label)} value={recoveryEnrollmentDraft[field]} maxLength={maxLength}
      style={styles.input} multiline={field==='document'} autoCorrect={false} autoCapitalize="none" editable={!busy}
      onChangeText={value=>setRecoveryEnrollmentDraft(previous=>({...previous,[field]:value}))} />)}
  </>;
  const routeReady = !!recoveryBoard || (!!recoveryOrigin &&
    !!recoveryEnrollmentDraft.document && /^[0-9a-f]{64}$/i.test(recoveryEnrollmentDraft.device.trim()) &&
    /^[0-9a-f]{64}$/i.test(recoveryEnrollmentDraft.fingerprint.trim()));
  const backToRecovery = language==='ko'?'복구 방법으로 돌아가기':'Back to recovery choices';
  // The personal wallet home carries the bottom tabs of the design book (지갑 · 결제 · 둘러보기 · 설정).
  const homeTabs = screen === 'owner' && isPersonal && !!stored?.verified && !boardWait;
  const pageTitle = t(screen === 'home' || (tabbed && !stored) ? 'NU Wallet' : screen === 'board' ? '내 보드' : screen in boardLifeTitles ? boardLifeTitles[screen as BoardLifeScreen] : screen === 'profiles' && tabbed ? '내 하드월렛을 연결해 주세요' : isOwner ? '개인 지갑' : screen === 'practice' ? '결제 연습' : settingsScreen ? (language === 'ko' ? ({'settings-access':'접근성','settings-diagnostics':'도움말·진단','settings-device':'기기·업데이트','credentials':'인증수단','travel-privacy':'개인정보와 여행 동의'} as Record<string,string>)[screen] ?? '설정'
        : ({'settings-access':'Accessibility','settings-diagnostics':'Help and diagnostics','settings-device':'Device and updates','credentials':'Sign-in methods','travel-privacy':'Privacy and travel consent'} as Record<string,string>)[screen] ?? 'Settings') : screen === 'install' ? '새 보드 설치' : screen === 'evm-registry' ? '네트워크 및 자산 설정' : screen === 'read-profiles' ? '6개 체인 · 읽기 전용' : screen === 'receive-chain' ? '다른 체인 받기' : screen === 'send-chain' ? '다른 체인 보내기' : screen === 'receive-evm' ? 'EVM 수신' : screen === 'transfer' ? '개인 전송' : screen === 'settings-device' || screen === 'policy-device' ? '기기·업데이트' : screen === 'policy-picker' ? '한도와 잠금' : screen === 'lock-picker' ? '기기 잠금 규칙 확인' : screen === 'firmware' ? '펌웨어 적용 증거 확인' : screen === 'dapp' ? 'DApp 둘러보기' : screen === 'walletconnect' ? '외부 DApp 연결' : screen === 'allowance' ? '온체인 토큰 권한' : screen === 'receipts' ? '지급 기록 관리' : screen === 'recovery-options' ? (language==='ko'?'어떤 기기를 교체하나요?':'Which device are you replacing?') : screen.startsWith('recovery-cloud') ? (language==='ko'?'클라우드 휴대전화 복구':'Cloud phone recovery') : screen === 'replace-phone' ? '분실한 휴대전화 교체' : screen === 'recovery' ? '지갑 복구' : screen === 'backup' ? '독립 백업' : '지갑 시작하기');
  const content=(
    <ScrollView ref={scroll} contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled"
      automaticallyAdjustKeyboardInsets={keyboardForm} keyboardDismissMode={keyboardForm ? 'on-drag' : 'none'}>
      {/* Verified means the board, its owner and the backed-up address were all checked; an enrollment
          document alone means the phone holds an owner credential but the account is not open yet. */}
      {isOwner && <Brand chip={stored?.verified ? {label:t('보드 연결'),tone:status?.state==='active'?'ok':'off'}
        : status?.state==='active' ? {label:t(status.root===2 ? '백업 확인 필요' : '서명 시험 필요'),tone:'wait'}
        : profile ? {label:t('보드 상태 미확인'),tone:'off'} : {label:t('보드 미등록'),tone:'wait'}} />}
      {tabbed && (isOwner || screen === 'home' || screen === 'profiles') ? <TabHeader title={pageTitle} settings={openSettings}
        label={language === 'ko' ? '설정' : 'Settings'} headingRef={sectionHeading} />
        : <Text ref={sectionHeading} accessibilityRole="header" style={styles.title}>{pageTitle}</Text>}
      <View style={styles.languageRow}><Chip label={language === 'ko' ? 'English' : '한국어'} onPress={busy || !stored || !native ? undefined : () => {
        const next = language === 'ko' ? 'en' : 'ko';
        if (stored) work(async () => { await save({...stored, language: next}); setLanguage(next); }, '언어 설정을 저장할 수 없습니다.');
      }} /></View>
      {!!message && <Text style={styles.body} accessibilityLiveRegion="polite">{t(message)}</Text>}
      {boardWait && <BoardStepPanel wait={boardWait} t={t} />}
      {displayBinding && <View>
        <Action title={showDisplayBinding ? (language==='ko'?'요청 정보 숨기기':'Hide private request') :
          (language==='ko'?'독립 표시 요청 정보 보기':'Show private request for independent display')}
          run={()=>setShowDisplayBinding(value=>!value)} />
        {displayCode && <>
          <Image accessibilityLabel={language==='ko'?'독립 표시용 일회용 QR':'One-time QR for the independent display'}
            source={{uri:receiveQr(displayCode)}} style={styles.ownerDisplayQr} />
          <Text testID="owner-display-request-code" style={styles.body}>{displayCode}</Text>
        </>}
      </View>}
      {screen==='owner' && Platform.OS==='android' && ownerPermissionDenied && <>
        <Action title={t('앱 권한 설정 열기')} disabled={busy} run={()=>work(()=>Linking.openSettings(),
          t('Android 설정에서 이 앱의 주변 기기 권한을 허용하세요.'))} />
        <Action title={t('개인 기기 검색 다시 시도')} disabled={busy} run={()=>scan()} />
      </>}
      {!stored && !message && <Text style={styles.body}>{t('지갑 기록 확인 중')}</Text>}
      {tabbed && stored && screen === 'owner' && isPersonal && boardRegistered && !stored.verified && status?.root !== 2 &&
        <Action title={t('보드 설정 이어서 하기')} disabled={busy} run={() => openLife('setup')} />}
      {stored && screen === 'home' && <>
        <BoardCard t={t} open={() => setScreen('board')}
          state={!stored.verified ? 'unverified' : boardStatusVerified ? 'active' : 'unchecked'} />
        {stored.spendingBoard ? <BoardBalance t={t} stored={stored} native={native} active={homeActive} orders={openOrders.length} hide={toggleBalance}
          deposit={() => openLife('deposit')} network={() => setScreen('read-profiles')} />
        : stored.verified && <WalletHomeSummary owner={stored.verified} state={stored.walletRead} language={language} active={homeActive}
          connectivity={connectivity} proof={readProof} onHide={toggleBalance}
          onAccounts={() => setScreen('read-profiles')} onReceive={() => setScreen('receive-chain')} />}
        {isPersonal && (stored.spendingBoard ? !testedBoard(stored)?.returnAddress : !stored.verified) &&
          <KitButton label={t('보드 설정 이어서 하기')} run={() => openLife('setup')} variant="secondary" />}
        <HomeOrders t={t} orders={openOrders} open={openOrder ?? orders} account={boardAccount} />
      </>}
      {stored && screen === 'board' && <BoardPage t={t} connected={status?.state === 'active'} spending={spending} permitNeeded={permitNeeded}
        setup={isPersonal && <SetupStatus stored={stored} hasEnrollment={!!profile} registered={boardRegistered} open={() => openLife('setup')} />}
        amount={units => formatReadAmount(units.toString(), 6) + ' TestUSDC'} back={() => setScreen(isPersonal ? 'home' : 'profiles')} actions={[
          {label: t('보드 연결·상태 확인'), detail: t('가까이 있는 내 보드를 찾아 상태를 읽습니다'), run: () => setScreen('owner')},
          ...(isPersonal ? [
            {label: t('입금'), detail: t('보드 계정의 주소와 QR을 봅니다'), run: () => openLife('deposit')},
            {label: t('반납'), detail: t('남은 돈을 원래 지갑으로 보내고 초기화합니다'), run: () => openLife('return')},
            {label: t('공장 초기화'), detail: t('PIN을 잊었을 때의 운영자 초기화도 여기 있습니다'), run: () => openLife('reset')},
          ] : []),
          {label: t('기기·업데이트'), run: () => { setMessage(''); setScreen('settings-device'); }},
          {label: t('원래 보드에서 분실한 휴대전화 교체'), run: () => setScreen('replace-phone')},
        ]} />}
      {stored && screen in boardLifeTitles && <BoardLife screen={screen as BoardLifeScreen} stored={stored} native={native} spending={spending}
        hasEnrollment={!!profile} registered={boardRegistered} found={found} busy={busy} scan={() => scan(!!stored.verified)} go={next => { setMessage(''); setScreen(next); }} orders={orders} work={work} say={sayLife}
        withBoard={boardRequest} wait={setBoardWait} copy={text => { Share.share({message: text}).catch(() => {}); }}
        commit={async change => {
          const next = await commit(change);
          if (!next.verified) { setStatus(undefined); setSeenBoard(undefined); setSpending(undefined); setVerifiedIdleLock(undefined); setPolicyHandle(''); }
        }} seenBoard={seenBoard} seen={(account, generation) => setSeenBoard({account, generation, at: Date.now()})} />}
      {stored && screen === 'profiles' && <>
        {tabbed && <FirstSteps t={t} connect={connectInstalledBoard} disabled={!native} />}
        {/* Event visitors try a payment on the operator's rental board without creating their own wallet. */}
        {Platform.OS === 'android' && <Card tone="green">
          <Text accessibilityRole="header" style={styles.trialTitle}>{t('행사에서 체험하기')}</Text>
          <Text style={styles.trialBody}>{t('운영자 안내에 따라 대여 보드에 연결하고 테스트넷 결제를 해 봅니다. 내 지갑은 만들지 않으며, 체험이 끝나면 운영자가 대여를 닫습니다.')}</Text>
          <KitButton label={t('체험 시작')} run={rental} variant="secondary" />
        </Card>}
        <Text style={styles.body}>{language === 'ko' ? '어떤 지갑을 사용할까요?' : 'Which wallet would you like to use?'}</Text>
        <View style={styles.device}><Text accessibilityRole="header" style={styles.sectionTitle}>{language === 'ko' ? '개인 보드' : 'Personal board'}</Text>
          <Text style={styles.body}>{language === 'ko' ? '내 보드를 새로 설치하거나 이미 설치된 보드를 연결합니다.' : 'Set up your board or connect one already installed.'}</Text>
          <Action title={language === 'ko' ? '새 보드 설치' : 'Set up a new board'} run={() => setScreen('install')} />
          <Action title={language === 'ko' ? '설치된 보드 연결' : 'Connect an installed board'} disabled={!native} run={connectInstalledBoard} /></View>
        <View style={styles.device}><Text accessibilityRole="header" style={styles.sectionTitle}>{language === 'ko' ? '기존 대여 보드' : 'Existing rental board'}</Text>
          <Text style={styles.body}>{language === 'ko' ? '기존 대여 주소와 잔액을 유지합니다.' : 'Keep your existing rental address and balance.'}</Text>
          {Platform.OS === 'android' ? <Action title={t('기기 찾기 · 기존 대여')} run={rental} /> : <Text style={styles.body}>{t('기존 대여는 등록에 사용한 Android 휴대전화에서 열 수 있습니다.')}</Text>}
          <Action title={language==='ko'?'대여 보드 반납 상태':'Rental board return'} run={()=>setScreen('rental-maintenance')}/></View>
        <Action testID="w06-choose-recovery" variant="row" title={language==='ko'?'분실 복구':'Wallet recovery'} run={()=>openRecoveryOptions('profiles')} />
        {showCloudWallet && <View style={styles.device}><Text accessibilityRole="header" style={styles.sectionTitle}>{language === 'ko' ? '클라우드 지갑' : 'Cloud wallet'}</Text>
          <Text style={styles.body}>{language === 'ko' ? '별도의 모바일 승인으로 사용하는 지갑입니다.' : 'A wallet with separate mobile approval.'}</Text>
          {cloudProfile ? <Action title={t('클라우드 지갑')} run={()=>setScreen('cloud')} /> : <Text style={styles.body}>{language === 'ko' ? '현재 이 기기에서 사용할 수 없습니다.' : 'Not available on this device yet.'}</Text>}</View>}
        <Action variant="row" title={stored.practice?.state === 'practice' ? (language === 'ko' ? '연습 이어하기' : 'Resume practice') : t('결제 연습')} run={() => {setPracticeWarning(false); setScreen('practice');}} />
      </>}
      {stored && screen === 'practice' && <>
        <Text style={styles.body}>{language === 'ko' ? '이 연습은 실제 돈을 보내거나 서명하지 않습니다. 보드에는 화면이 없습니다. 실제 결제는 신뢰할 수 있는 별도 표시에서 내용을 확인한 뒤 보드 버튼으로 승인합니다.' : 'This practice sends no money and creates no signature. The board has no screen. For a real payment, review the details on a trusted independent display before using the board button.'}</Text>
        {!stored.practice && <>
          <Text style={styles.body}>{language === 'ko' ? '먼저 연습할 주문을 선택하세요.' : 'Choose a practice order.'}</Text>
          <Action title={language === 'ko' ? '같은 금액 비교하기' : 'Compare matching amounts'} disabled={!native || busy} run={() => work(() => save({...stored,practice:beginPractice('matching')}), '연습 기록을 저장할 수 없습니다.')} />
          <Action title={language === 'ko' ? '틀린 금액 찾아보기' : 'Find a wrong amount'} disabled={!native || busy} run={() => work(() => save({...stored,practice:beginPractice('wrongAmount')}), '연습 기록을 저장할 수 없습니다.')} />
        </>}
        {stored.practice?.state === 'practice' && <>
          <View style={styles.device} accessible accessibilityLabel={language === 'ko' ? `주문: ${practiceOrder.recipient}, ${practiceOrder.amount}. 비교 화면: ${practiceOrder.recipient}, ${practiceOrder.comparisonAmount}.` : `Order: ${practiceOrder.recipient}, ${practiceOrder.amount}. Comparison display: ${practiceOrder.recipient}, ${practiceOrder.comparisonAmount}.`}>
            <Text accessibilityRole="header" style={styles.sectionTitle}>{language === 'ko' ? '주문 내용' : 'Order'}</Text>
            <Text style={styles.body}>{practiceOrder.recipient} · {practiceOrder.amount}</Text>
            <Text accessibilityRole="header" style={styles.sectionTitle}>{language === 'ko' ? '비교할 표시' : 'Comparison display'}</Text>
            <Text style={styles.body}>{practiceOrder.recipient} · {practiceOrder.comparisonAmount}</Text>
          </View>
          <Text style={styles.body}>{language === 'ko' ? '상점과 금액이 같나요? 다르면 거절하세요.' : 'Do the merchant and amount match? Decline if they differ.'}</Text>
          {practiceWarning && <Text accessibilityLiveRegion="polite" style={styles.body}>{language === 'ko' ? '금액이 다릅니다. 이 요청은 거절하세요.' : 'The amounts differ. Decline this request.'}</Text>}
          {(['approve','reject'] as const).map(choice => <Action key={choice} title={choice === 'approve' ? (language === 'ko' ? '연습 승인' : 'Approve practice') : (language === 'ko' ? '연습 거절' : 'Decline practice')} disabled={busy} run={() => {
            const next = practiceDecision(stored.practice as Practice, choice);
            if (next.state === 'practice') {setPracticeWarning(true); return;}
            work(() => save({...stored,practice:next,exercised:true}), '연습 기록을 저장할 수 없습니다.');
          }} />)}
        </>}
        {stored.practice && stored.practice.state !== 'practice' && <>
          <Text accessibilityRole="header" style={styles.sectionTitle}>{stored.practice.state === 'done' ? (language === 'ko' ? '연습을 승인했습니다' : 'Practice approved') : (language === 'ko' ? '연습 주문을 거절했습니다' : 'Practice declined')}</Text>
          <Text style={styles.body}>{language === 'ko' ? '실제 지급 0건 · 서명 0건' : 'Real payments: 0 · Signatures: 0'}</Text>
          {stored.practice.state === 'reject' && <>
            <Text style={styles.body}>{language === 'ko' ? '실제 주문의 금액이 다르면 요청을 닫고 새 견적을 받으세요.' : 'If a real amount differs, decline the request and get a new quote.'}</Text>
            <Action title={language === 'ko' ? '같은 연습 다시 보기' : 'Replay the same practice'} disabled={!native || busy}
              run={() => work(() => save({...stored,practice:beginPractice(stored.practice!.scenario)}),
                language === 'ko' ? '연습 기록을 저장할 수 없습니다.' : 'Could not save the practice result.')} />
            <Action title={language === 'ko' ? '실물 보드 등록' : 'Register a physical board'} disabled={!native || busy} run={connectInstalledBoard} />
          </>}
          <Action title={language === 'ko' ? '다른 연습 선택' : 'Choose another practice'} disabled={busy} run={() => work(() => save({...stored,practice:undefined}), '연습 기록을 저장할 수 없습니다.')} />
        </>}
        <Action title={language === 'ko' ? '나중에 이어하기' : 'Continue later'} disabled={busy} run={() => setScreen(isPersonal ? 'owner' : 'profiles')} />
      </>}
      {stored && screen === 'install' && <>
        <Text accessibilityRole="header" style={styles.sectionTitle}>{stored.installation?.state === 'consumed' ? (language === 'ko' ? '설치 확인서를 검증했습니다' : 'Installation receipt verified') : (language === 'ko' ? '설치 검증을 기다리고 있습니다' : 'Installation verification pending')}</Text>
        {/* Three plain steps replace the release-engineering paragraphs; the detailed checks stay in the setup tool. */}
        <Card>
          {(language === 'ko' ? ['컴퓨터에 보드를 USB로 연결하고 설치 도구로 이미지를 기록하세요.', '설치 도구가 만든 검증 파일과, 따로 받은 신뢰 파일을 차례로 고르세요.', '파일의 값이 따로 받은 기록과 같은지 확인한 뒤 설치 확인 요청을 만드세요.']
            : ['Connect the board to a computer by USB and write the image with the setup tool.', 'Choose the verified file from the setup tool, then the trust file you received separately.', 'Check the values against the records you received separately, then create the installation request.'])
            .map((step, index) => <Text key={step} style={styles.body}>{`${index + 1}. ${step}`}</Text>)}
        </Card>
        {!stored.installation && <>
          <Action title={language === 'ko' ? '검증된 설치 파일 선택' : 'Choose verified setup file'} disabled={busy || !native} run={() => work(async()=>{
            if (!native) throw new Error('NATIVE_REQUIRED');
            const text=await native.pickInstallationFile(); if (text.length > 8192) throw new Error('FILE_TOO_LARGE');
            const data=JSON.parse(text) as InstallPreflight;
            if (!data || typeof data !== 'object') throw new Error('FILE_INVALID');
            setInstallConfirmed(false); setInstallSetup(text);
          },language === 'ko' ? '설치 파일을 선택하지 못했습니다.' : 'Could not select the setup file.')} />
          <Action variant="secondary" title={language === 'ko' ? '독립 신뢰 파일 선택' : 'Choose independent trust file'} disabled={busy || !native} run={() => work(async()=>{
            if (!native) throw new Error('NATIVE_REQUIRED');
            const trust=readInstallTrust(await native.pickInstallationFile());
            setInstallConfirmed(false); setInstallerPin(trust.signer); setCandidatePin(trust.candidate); setInstallDevice(trust.device);
          },language === 'ko' ? '독립 신뢰 파일을 확인할 수 없습니다.' : 'Could not verify the independent trust file.')} />
          <Action variant="row" title={language === 'ko' ? (installAdvanced ? '직접 입력 닫기' : '직접 입력하기') : (installAdvanced ? 'Close manual entry' : 'Enter manually')} run={()=>{setInstallAdvanced(!installAdvanced); setInstallConfirmed(false);}} />
          {installAdvanced && <>
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={language === 'ko' ? '검증된 설치 자료' : 'Verified setup data'} value={installSetup} onChangeText={value=>{setInstallConfirmed(false); setInstallSetup(value);}} multiline maxLength={4096} autoCapitalize="none" autoCorrect={false} />
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={language === 'ko' ? '독립 설치자 지문' : 'Independent installer fingerprint'} value={installerPin} onChangeText={value=>{setInstallConfirmed(false); setInstallerPin(value);}} maxLength={64} autoCapitalize="none" autoCorrect={false} />
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={language === 'ko' ? '독립 릴리스 해시' : 'Independent release hash'} value={candidatePin} onChangeText={value=>{setInstallConfirmed(false); setCandidatePin(value);}} maxLength={64} autoCapitalize="none" autoCorrect={false} />
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={language === 'ko' ? '독립 보드 식별값' : 'Independent board identity'} value={installDevice} onChangeText={value=>{setInstallConfirmed(false); setInstallDevice(value);}} maxLength={64} autoCapitalize="none" autoCorrect={false} />
          </>}
          {!!installSetup && !!installerPin && !!candidatePin && !!installDevice && <View style={styles.device}>
            <Text accessibilityRole="header" style={styles.sectionTitle}>{language === 'ko' ? '독립 기록과 비교' : 'Compare with independent records'}</Text>
            <Text style={styles.body}>{language === 'ko' ? '선택한 파일만 믿지 말고 별도 경로의 값과 각각 대조하세요.' : 'Compare each value against records obtained independently of these files.'}</Text>
            <Text selectable style={styles.mono}>{language === 'ko' ? '설치자 지문: ' : 'Installer fingerprint: '}{installerPin}</Text>
            <Text selectable style={styles.mono}>{language === 'ko' ? '릴리스 해시: ' : 'Release hash: '}{candidatePin}</Text>
            <Text selectable style={styles.mono}>{language === 'ko' ? '보드 식별값: ' : 'Board identity: '}{installDevice}</Text>
            <Action title={language === 'ko' ? '독립 기록과 일치함' : 'Values match independent records'} disabled={busy} run={()=>setInstallConfirmed(true)} />
            {installConfirmed && <Text style={styles.body}>{language === 'ko' ? '비교를 확인했습니다.' : 'Comparison confirmed.'}</Text>}
          </View>}
          <Action title={language === 'ko' ? '설치 확인 요청 만들기' : 'Create installation challenge'} disabled={busy || !native || !installConfirmed} run={() => work(async()=>{
            if (!native || !installConfirmed) throw new Error('NATIVE_REQUIRED');
            const challenge=bytesToHex(fromBase64(await native.randomBytes(32)),false);
            const checkpoint=startInstallation(JSON.parse(installSetup) as InstallPreflight,installerPin.trim(),candidatePin.trim(),installDevice.trim(),0,challenge,readInstallClock(await native.installationClock()));
            await save({...stored,installation:checkpoint}); setInstallSetup(''); setInstallerPin(''); setCandidatePin(''); setInstallDevice(''); setInstallConfirmed(false);
          },language === 'ko' ? '설치 자료 또는 독립 지문이 일치하지 않습니다.' : 'Setup data and independent fingerprints do not match.')} />
        </>}
        {stored.installation?.state === 'pending' && <>
          <Text style={styles.body}>{language === 'ko' ? '이 요청값을 설치자에게 전달하고, 서명된 실제 설치 확인서를 가져오세요. 이 요청은 5분 동안 유효합니다.' : 'Give this challenge to the installer and import the signed observed installation receipt. This challenge lasts five minutes.'}</Text>
          <Text selectable style={styles.mono}>{stored.installation.expected.challenge}</Text>
          <Action title={language === 'ko' ? '서명된 확인서 파일 선택' : 'Choose signed receipt file'} disabled={busy || !native} run={() => work(async()=>{
            if (!native) throw new Error('NATIVE_REQUIRED');
            const text=await native.pickInstallationFile(); if (text.length > 8192) throw new Error('FILE_TOO_LARGE');
            JSON.parse(text); setInstallReceipt(text);
          },language === 'ko' ? '확인서 파일을 읽을 수 없습니다.' : 'Could not read the receipt file.')} />
          <Action title={language === 'ko' ? (installAdvanced ? '직접 입력 닫기' : '확인서 직접 입력') : (installAdvanced ? 'Close manual entry' : 'Enter receipt manually')} run={()=>setInstallAdvanced(!installAdvanced)} />
          {installAdvanced && <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={language === 'ko' ? '서명된 설치 확인서' : 'Signed installation receipt'} value={installReceipt} onChangeText={setInstallReceipt} multiline maxLength={8192} autoCapitalize="none" autoCorrect={false} />}
          {!!installReceipt && <Action title={language === 'ko' ? '확인서 지우기' : 'Clear receipt'} disabled={busy} run={() => setInstallReceipt('')} />}
          <Action title={language === 'ko' ? '설치 확인서 검증' : 'Verify installation receipt'} disabled={busy || !installReceipt} run={() => work(async()=>{
            const checkpoint=stored.installation;
            if (!checkpoint || checkpoint.state !== 'pending') throw new Error('INSTALL_STATE');
            if (!native) throw new Error('NATIVE_REQUIRED');
            const next=consumeInstallation(checkpoint,JSON.parse(installReceipt),readInstallClock(await native.installationClock()));
            await save({...stored,installation:next}); setInstallReceipt('');
          },language === 'ko' ? '확인서가 유효하지 않거나 요청이 만료됐습니다.' : 'The receipt is invalid or the request has expired.')} />
        </>}
        {stored.installation?.state === 'consumed' && <>
          <Text style={styles.body}>{language === 'ko' ? '서명된 설치 확인서를 검증했습니다. 같은 보드에 연결해 상태를 다시 확인합니다.' : 'The signed installation receipt was verified. Connect to the same board to recheck its state.'}</Text>
          <Action title={language === 'ko' ? '같은 보드 등록 계속' : 'Continue with this board'} disabled={busy} run={() => work(async()=>{
            const checkpoint=stored.installation;
            if (!checkpoint || checkpoint.state !== 'consumed' || !native) throw new Error('INSTALL_EXPIRED');
            const now=installationTime(checkpoint.clock,readInstallClock(await native.installationClock()));
            if (now >= checkpoint.claim.expiresAt) throw new Error('INSTALL_EXPIRED');
            setFromInstall(true); setScreen('owner');
          },language === 'ko' ? '설치 확인이 만료됐거나 기기가 재부팅되었습니다. 요청을 취소하고 다시 시작하세요.' : 'Installation confirmation expired or the device rebooted. Cancel the request and start again.')} />
        </>}
        {!!stored.installation && <Action title={language === 'ko' ? '설치 요청 취소' : 'Cancel installation request'} disabled={busy} run={() => work(()=>save({...stored,installation:undefined}),language === 'ko' ? '설치 요청을 취소할 수 없습니다.' : 'Could not cancel the installation request.')} />}
        <Action variant="secondary" title={language === 'ko' ? '지갑 선택으로' : 'Back to wallet selection'} run={() => setScreen('profiles')} />
      </>}
      {stored && isOwner && <>
        {isPersonal && stored.verified && <WalletHomeSummary owner={stored.verified} state={stored.walletRead} language={language} active={homeActive}
          connectivity={connectivity} proof={readProof}
          onHide={toggleBalance}
          onAccounts={() => setScreen('read-profiles')} onReceive={() => setScreen('receive-chain')} />}
        {isPersonal && stored.verified && <View style={styles.pair}>
          <View style={styles.pairItem}><Action title={t('다른 체인 받기')} disabled={busy} variant="secondary" run={() => setScreen('receive-chain')} /></View>
          <View style={styles.pairItem}><Action title={t('다른 체인 보내기')} disabled={busy} variant="secondary" run={() => setScreen('send-chain')} /></View>
        </View>}
        {isPersonal && !tabbed && !homeTabs && <Action title={language === 'ko' ? '설정' : 'Settings'} disabled={busy} run={openSettings} variant="secondary" />}
        {demoAllowed && !stored.verified && <Action title={t('시연용 지갑 화면 보기')} variant="row" disabled={busy} run={() => setScreen('demo')} />}
        {!stored.verified && (tabbed ? <Card>
          <Text accessibilityRole="header" style={styles.sectionTitle}>{t('보드 연결하기')}</Text>
          {['이 휴대전화의 등록 공개키를 발급 담당자에게 보여 주고 등록서를 받으세요.',
            '등록서와 설치 기록의 기기 식별값·서버 공개키 지문을 입력하세요.',
            '아래 하드월렛 단계에 따라 보드를 준비한 뒤 개인 기기를 찾아 등록하세요.'].map((step, index) =>
            <Text key={step} style={styles.body}>{`${index + 1}. ${t(step)}`}</Text>)}
        </Card> : <>
        {!stored.verified && <Text style={styles.body}>{language === 'ko' ? '독립 설치 기록에서 보드 지문을 확인한 뒤 Bluetooth 권한을 허용하세요. 이 보드에는 화면이 없습니다. 보드 버튼으로 승인하고 PIN을 입력합니다.' : 'Check the board fingerprint against independent installation records before allowing Bluetooth. The board has no screen. Approve and enter the PIN with the board buttons.'}</Text>}
        {!stored.verified && <Text style={styles.body}>{t('발급 담당자에게 휴대전화 공개키를 건네세요. 독립 확인이 가능한 공급·설치 기록에서 기기 식별값과 서버 공개키 지문을 대조하세요.')}</Text>}
        </>)}
        <Action title={t('휴대전화의 등록 공개키 보기')} variant="row" disabled={!native || busy} run={() => work(async () => {
          if (!native) throw new Error('SECURE_IDENTITY_REQUIRED');
          setPublicKey(await native.publicIdentity());
        }, '이 휴대전화에서 보호된 등록 키를 만들 수 없습니다.')} />
        {!!publicKey && <Text selectable style={styles.mono}>{publicKey}</Text>}
        {!profile && <>
          {commissioningPins && <>
            <Text style={styles.body}>{language === 'ko' ? '케이블로 확인한 독립 설치 QR의 지문을 대조하세요.' : 'Compare both fingerprints with the independent cabled installation record.'}</Text>
            <Text selectable style={styles.mono}>{t('기기 식별값')}: {commissioningPins.device}</Text>
            <Text selectable style={styles.mono}>{t('발급 문서의 서버 공개키 지문')}: {commissioningPins.boardSPKI}</Text>
            <Action title={language === 'ko' ? '대조한 설치 지문 사용' : 'Use compared installation fingerprints'} disabled={busy} run={() => {
              setBoard(commissioningPins.device);
              setPin(commissioningPins.boardSPKI);
              dismissCommissioningPins?.();
            }} />
            <Action title={language === 'ko' ? '설치 QR 닫기' : 'Dismiss installation QR'} run={() => dismissCommissioningPins?.()} />
          </>}
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('발급 등록서')} placeholder={t('발급 등록서')} value={raw} onChangeText={setRaw} maxLength={8000} multiline autoCorrect={false} autoCapitalize="none" />
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('기기 식별값')} placeholder={t('기기 식별값')} value={board} onChangeText={setBoard} maxLength={64} autoCorrect={false} autoCapitalize="none" />
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('발급 문서의 서버 공개키 지문')} placeholder={t('발급 문서의 서버 공개키 지문')} value={pin} onChangeText={setPin} maxLength={64} autoCorrect={false} autoCapitalize="none" />
        </>}
        {/* The board step comes after the enrollment inputs, right before the scan it prepares. */}
        {!stored?.verified && status?.state !== 'active' && !boardWait && <BoardStepPanel wait={{flow:'register',step:'window'}} t={t} />}
        <Action title={t('개인 기기 찾기')} disabled={!native || busy} run={()=>scan()} />
        {ownerScanComplete && !found.length && <Text style={styles.body} accessibilityLiveRegion="polite">
          {t('기기 검색을 마쳤습니다. 주변에서 개인 보드를 찾지 못했습니다.')}
        </Text>}
        {found.map((item, index) => <View key={item.address} style={styles.device}>
          <Text style={styles.body}>{item.name || t('이름이 표시되지 않는 기기')} · {language === 'ko' ? '검색 후보' : 'Candidate'} {index + 1} · {item.address.slice(-6)}</Text>
          <Text style={styles.body}>{language === 'ko' ? '검색 이름과 끝자리는 지문이 아닙니다. 독립 기록과 인증 연결로 확인하세요.' : 'The scan name and suffix are not a fingerprint. Verify using independent records and an authenticated connection.'}</Text>
          <Action title={t('등록 상태 확인')} disabled={busy} run={() => open(item.address, 'status')} />
          {isPersonal && stored.verified && <Action title={language === 'ko' ? '보호 정책 읽기' : 'Read device policy'} disabled={busy}
            run={() => refreshPolicy(item.address)} />}
          {retiring && <Action title={t('기기에서 이 휴대전화 권한 철회')} disabled={busy} run={() => open(item.address,'revoke')} />}
          <Action title={t('기기 등록 시작')}  disabled={busy || status?.state === 'active'} run={() => open(item.address, 'register')} />
        </View>)}
        {status?.state === 'active' && stored.verified?.boardFingerprint === status.device &&
          BigInt(stored.verified.generation) === status.generation && <>
            <Text style={styles.body}>{t('확인된 수신 주소 · StableNet')} {stored.verified.network}</Text>
            <Text selectable style={styles.mono}>{stored.verified.address}</Text>
          </>}
        {status?.state === 'active' && <Text style={styles.body}>{t('소유자 세대')} {status.generation.toString()}. {t('개인 계정과 결제 승인은 기기와 복구 증거가 별도로 확인되어야 열립니다.')}</Text>}
        {status?.state === 'active' && !retiring && <Action title={t('휴대전화 관리 권한 철회')} variant="secondary" disabled={busy} run={() => {
          setRetiring(true);
          setMessage('철회하면 이 휴대전화에서 기기를 관리할 수 없습니다. 자산 복구 경로를 먼저 확인하세요.');
        }} />}
        {retiring && <Action title={t('권한 철회 취소')} disabled={busy} run={() => {setRetiring(false); setMessage('');}} />}
        {status?.state === 'active' && status.root === 2 && status.profile === 1 && status.backedUp &&
          <Text style={styles.body}>{t('원본 기기가 독립 복원 시험 완료를 확인했습니다.')}</Text>}
        {!stored.verified && !tabbed && <>
          <Action title={t('다른 체인 받기')} disabled variant="row" run={() => setScreen('receive-chain')} />
          <Action title={t('다른 체인 보내기')} disabled variant="row" run={() => setScreen('send-chain')} />
        </>}
        {stored.signing && <>
          <Text style={styles.body}>{language === 'ko' ?
            (stored.signing.kind==='evm' ? '이전 EVM 서명 결과를 확인하지 못했습니다. 요청 만료만으로 서명이 무효가 되지는 않습니다. 원래 기기와 요청을 보관하고 신뢰된 정산 증거를 확인하세요.' : '이전 승인 결과를 저장하지 못했습니다. 원래 보드에서 승인 요청이 만료됐는지 확인하세요.') :
            (stored.signing.kind==='evm' ? 'The earlier EVM signing result is unknown. Request expiry does not invalidate a signature. Keep the original device and request and seek trusted reconciliation.' : 'The previous approval outcome is unresolved. Verify expiry on the original board.')}</Text>
          {stored.signing.kind==='evm' && stored.signing.evm && <>
            <Text style={styles.body}>{language==='ko'?'보관한 원문은 미결 확인용입니다. 서명 발급 여부는 확인되지 않았습니다.':'The saved original is for reconciliation. Whether a signature was issued is unknown.'}</Text>
            <Action title={language==='ko'?'미결 요청 내보내기':'Export unresolved request'} disabled={busy} run={()=>work(async()=>{
              const signing=currentStorage.current?.signing;if(!signing || signing.kind!=='evm' || !signing.evm)throw new Error('EVM_PENDING_MISSING');
              await Share.share({message:JSON.stringify({type:'nu54-unresolved-evm-v1',board:signing.board,generation:signing.generation,expires:signing.expires,...signing.evm},null,2)});
            },language==='ko'?'요청을 내보내지 못했습니다.':'Could not export the request.')} />
          </>}
          {stored.signing.kind==='chain' && stored.verified && found.map(item => <Action key={'signing-' + item.address}
            title={(language === 'ko' ? '만료된 승인 확인' : 'Verify expired approval') + ' · ' + item.name} disabled={busy}
            run={() => work(async () => {
              const {signing, verified} = stored;
              if (!signing || !verified || signing.board !== verified.boardFingerprint || signing.generation !== verified.generation)
                throw new Error('WALLET_SIGNING_BINDING');
              await chainSession(item.address, session => session.confirmSigningExpiry(signing, verified.address));
              await save({...stored, signing:undefined});
            }, language === 'ko' ? '원래 보드가 승인 요청 만료를 확인하지 못했습니다.' : 'The original board could not verify expiry.')} />)}
        </>}
        {tabbed ? stored.verified && <>
        <Section title={t('자산과 계정')}>
          <Action variant="row" detail={t('비트코인·이더리움 등 여러 네트워크의 잔액을 봅니다')} title={t('6개 체인 · 읽기 전용')} disabled={busy || !stored.verified}
          run={() => {setTransfer(undefined); setTransferWire(''); setScreen('read-profiles');}} />
          <Action variant="row" detail={t('이더리움 계열 네트워크의 주소로 받습니다')} title={t('EVM 수신')} disabled={busy || !stored.verified} run={() => {setReceiveAddress(''); setEvmReceiveBalance(undefined);setScreen('receive-evm');}} />
          <Action variant="row" detail={t('등록한 주소로 테스트 자산을 보냅니다')} title={t('개인 전송')} disabled={busy || !stored.verified} run={() => setScreen('transfer')} />
          <Action variant="row" detail={t('결제 영수증과 지급 기록을 봅니다')} title={t('지급 기록 관리')} disabled={busy || !profile} run={() => setScreen('receipts')} />
        </Section>
        </> : <>
        <Section title={t('자산과 네트워크')}>
          <Action variant="row" detail={t('비트코인·이더리움 등 여러 네트워크의 잔액을 봅니다')} title={t('6개 체인 · 읽기 전용')} disabled={busy || !stored.verified}
          run={() => {setTransfer(undefined); setTransferWire(''); setScreen('read-profiles');}} />
          <Action variant="row" detail={t('이더리움 계열 네트워크의 주소로 받습니다')} title={t('EVM 수신')} disabled={busy || !stored.verified} run={() => {setReceiveAddress(''); setEvmReceiveBalance(undefined);setScreen('receive-evm');}} />
          <Action variant="row" title={t('네트워크 및 자산 설정')} disabled={busy || !stored.verified} run={() => setScreen('evm-registry')} />
          <Action variant="row" detail={t('등록한 주소로 테스트 자산을 보냅니다')} title={t('개인 전송')} disabled={busy || !stored.verified} run={() => setScreen('transfer')} />
          <Action variant="row" detail={t('결제 영수증과 지급 기록을 봅니다')} title={t('지급 기록 관리')} disabled={busy || !profile} run={() => setScreen('receipts')} />
        </Section>
        <Section title={t('앱 연결과 권한')}>
          <Action variant="row" title={t('외부 DApp 연결')} disabled={busy || !stored.verified || !registry?.walletConnect || !registryCurrent}
          run={()=>openExternalConnection(connections[0])} />
          <Action variant="row" title={t('DApp 둘러보기')} disabled={busy || !stored.verified} run={() => {dappSigned.current=undefined;setScreen('dapp');}} />
          <Action variant="row" title={t('온체인 토큰 권한')} disabled={busy || !stored.verified} run={()=>openAllowance()} />
        </Section>
        <Section title={t('보안과 복구')}>
          <Action variant="row" title={t('독립 백업 시작')} disabled={busy || !profile} run={() => setScreen('backup')} />
          <Action variant="row" title={t('원래 보드에서 분실한 휴대전화 교체')} disabled={busy} run={() => setScreen('replace-phone')} />
          <Action variant="row" title={t('휴대전화 분실 및 지갑 복구')} disabled={busy} run={() => setScreen('recovery')} />
          <Action variant="row" title={t('펌웨어 적용 증거 확인')} disabled={busy || !profile} run={() => setScreen('firmware')} />
        </Section>
        <Section title={t('연습')}>
          <Action variant="row" title={t('결제 연습')} disabled={busy} run={() => {setPracticeWarning(false); setScreen('practice');}} />
        </Section>
        </>}
        {(!isPersonal || fromInstall) && <Action variant="secondary" title={t('이전')} disabled={busy} run={() => {setFromInstall(false); setScreen('profiles');}} />}
      </>}
      {stored && settingsScreen && <WalletSettings tabbed={tabbed} language={language}
        policyRoute={screen==='policy'||screen==='policy-applied'} applied={screen==='policy-applied'}
        section={screen==='settings'?'home':screen==='settings-access'?'access':screen==='settings-diagnostics'?'diagnostics':undefined}
        accessibility={()=>setScreen('settings-access')} diagnostics={()=>setScreen('settings-diagnostics')}
        boardPage={tabbed&&screen==='settings'?()=>setScreen('board'):undefined}
        recover={screen==='settings'?()=>openRecoveryOptions('settings'):undefined}
        backup={tabbed&&screen==='settings'&&profile?()=>setScreen('backup'):undefined}
        replacePhone={tabbed&&screen==='settings'?()=>setScreen('replace-phone'):undefined}
        firmwareEvidence={tabbed&&screen==='settings'&&profile?()=>setScreen('firmware'):undefined}
        assetNetworks={tabbed&&screen==='settings'&&stored.verified?()=>setScreen('evm-registry'):undefined}
        credentials={screen==='settings'?()=>setScreen('credentials'):undefined}
        networks={screen==='settings'&&stored.verified?()=>setScreen('read-profiles'):undefined}
        privacy={screen==='settings'?()=>setScreen('travel-privacy'):undefined}
        idleLock={verifiedIdleLock?.board===stored.verified?.boardFingerprint && verifiedIdleLock?.generation===stored.verified?.generation ? verifiedIdleLock?.rule : undefined}
        checkIdleLock={stored.verified&&(screen==='settings'||screen==='policy') ? ()=>openDeviceVerification('lock') : undefined}
        setLanguage={next => work(async () => {await save({...stored, language:next}); setLanguage(next);}, language === 'ko' ? '언어 설정을 저장할 수 없습니다.' : 'Language preference could not be saved.')}
        back={() => {if(screen==='settings'){setMessage('');setScreen(tabbed?'home':'owner');}else openSettings();}} practice={() => {setPracticeWarning(false); setScreen('practice');}}
        rejectPractice={native ? rejectAccessibilityPractice : undefined} resetAccessibilityScroll={resetAccessibilityScroll} pendingOrders={orders} pendingEvm={() => setScreen('transfer')} pendingChain={() => setScreen('send-chain')} device={() => {setMessage('');setScreen(screen==='policy'?'policy-device':'settings-device');}}
        pending={{evm:!!stored.pendingEvm, chain:!!stored.pendingChain, signing:!!stored.signing, evmExpires:stored.pendingEvm?.expires}}
        policy={screen==='settings'?undefined:policyView} policyFresh={screen==='policy'&&policyFresh} policyFinalityFresh={screen==='policy'&&policyFinalityFresh} policyFinalityPending={screen==='policy'&&policyFinalityPending} policyDecimals={policyDecimals} policyReadAt={policyReadAt}
        finalityEnroll={()=>finalityFile('enrollment')} finalityReconcile={()=>finalityFile('evidence')} finalityResume={resumeFinality}
        freshAsset={freshPolicyAsset ? {chain:'8283',address:freshPolicyAsset.contract,decimals:freshPolicyAsset.decimals} : undefined}
        chooseAsset={() => setScreen('read-profiles')} initialize={initializePolicy} change={policyRefreshRequired ? undefined : changePolicy} activate={activatePolicy}
        changeSet={policyRefreshRequired ? undefined : changeSetPolicy}
        policyContacts={stored.evmRecipients?.filter(entry=>entry.genesis===visibleEvmNetworks.find(network=>policyView?.chain.toString()===network.chain)?.genesis)}
        readPolicy={stored.verified ? ()=>openDeviceVerification('policy') : undefined}
        boardStatus={boardStatusVerified?'active':'unavailable'}
        boardSuffix={boardStatusVerified?verifiedWallet?.boardFingerprint.slice(-4).toUpperCase():undefined} />}
      {stored && (screen==='policy-picker'||screen==='lock-picker') && <>
        <Text accessibilityRole="header" style={styles.sectionTitle}>{language==='ko'?'등록 기기 선택':'Choose the enrolled device'}</Text>
        <Text style={styles.body}>{language==='ko'?'선택한 기기의 인증이 완료되면 요청한 설정을 확인합니다.':'The app authenticates the selected board before reading the requested setting.'}</Text>
        <Action title={language==='ko'?'등록 기기 찾기':'Find enrolled device'} disabled={busy} run={()=>scan(true)} />
        {found.map((item,index)=><Action key={item.address} disabled={busy}
          title={(language==='ko'?'등록 기기 인증':'Authenticate enrolled device')+' · '+(index+1)+' · '+item.name}
          run={()=>screen==='policy-picker'?refreshPolicy(item.address):checkOwnerIdleLock(item.address,true)} />)}
        <Action title={language==='ko'?'설정으로 돌아가기':'Return to Settings'} disabled={busy} run={()=>{setMessage('');setScreen('settings');}} />
      </>}
      {stored && screen === 'receipts' && <>
        <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('발급자가 서명한 정산 결과')} value={reclaimWire}
          onChangeText={setReclaimWire} maxLength={1510} multiline autoCorrect={false} autoCapitalize="none" />
        <Action title={t('개인 기기 찾기')} disabled={!profile || busy} run={()=>scan()} />
        {found.map(item => <Action key={item.address} title={t('확정된 지급 기록 회수')} disabled={!reclaimWire || busy}
          run={() => reclaimReceipt(item.address)} />)}
        <Action title={t('등록 상태로 돌아가기')} disabled={busy} run={() => setScreen('owner')} />
      </>}
      {stored?.verified && native && (screen === 'receive-chain' || screen === 'send-chain' || screen === 'read-profiles') &&
        <ChainWalletScreen key={screen+stored.verified.boardFingerprint+stored.verified.generation} language={language} initial={screen === 'read-profiles' ? 'profiles' : screen === 'receive-chain' ? 'receive' : 'send'} verified={stored.verified} preferred={stored.chainSelection}
          readState={stored.walletRead} saveRead={walletRead=>save({...stored,walletRead})} connectivity={connectivity} onReadProof={setReadProof} pendingEvm={stored.pendingEvm}
          onOpenPending={() => setScreen('transfer')}
          onWatchSignet={commissioned.some(row=>row.chain===1) ? account=>work(async()=>{
            if (stored.walletRead?.disabled?.includes('btc')) throw new Error('CHAIN_COMMISSIONED_SCOPE');
            await save({...stored,chainSelection:{chain:1,network:2,account}});
            setScreen('receive-chain');
          },t('이 네트워크의 승인 프로필을 사용할 수 없습니다.')) : undefined}
          pickBip48File={()=>native.pickPsbtFile()} pendingBip48={stored.pendingBip48} persistBip48={async(wire,expires,approve)=>{
            await persistWalletSignature(stored,'bip48',wire,expires,commit,async()=>({pendingBip48:await approve()}));
          }}
          savedWatch={stored.bitcoinWatch} saveWatch={bitcoinWatch=>save({...stored,bitcoinWatch})}
          commissioned={commissioned} commissionedToken={stored.evmRegistry?.token}
          checkCommissioned={(scope,key,revision)=>{
            const readId=({1:'btc',2:'sol',3:'xrp'} as const)[scope.chain];
            if (stored.walletRead?.disabled?.includes(readId) ||
                !currentCommissioned(key,revision).some(row=>row.chain===scope.chain && row.anchor===scope.anchor && row.rpc===scope.endpoint))
              throw new Error('CHAIN_COMMISSIONED_SCOPE');
          }}
          reviewableNetworks={(Object.keys(readProfiles) as (keyof typeof readProfiles)[]).filter(id=>{
            const candidate=readProfiles[id];
            return 'evm' in candidate ?
              registryCurrent && visibleEvmNetworks.some(network=>network.chain===candidate.evm) :
              commissioned.some(network=>network.chain===candidate.chain);
          })}
          onReviewNetwork={id=>work(async()=>{
            const selected=readProfiles[id];
            if ('evm' in selected) {
              if (!registryCurrent || !visibleEvmNetworks.some(network=>network.chain===selected.evm))
                throw new Error('WALLET_REVIEW_NOT_ENABLED');
              setReceiveChain(selected.evm); setEvmAsset(''); setTransfer(undefined); setTransferWire('');
              setScreen('transfer');
              return;
            }
            if (!commissioned.some(row=>row.chain===selected.chain)) throw new Error('CHAIN_COMMISSIONED_SCOPE');
            await save({...stored,chainSelection:{chain:selected.chain,network:2,account:
              stored.walletRead?.selection.id===id ? stored.walletRead.selection.account : 0}});
            setScreen('send-chain');
          },t('이 네트워크의 승인 프로필을 사용할 수 없습니다.'))}
          choose={chainSelection=>save({...stored,chainSelection})} found={found} scan={()=>scan()} back={() => setScreen('owner')} busy={busy} pending={stored.pendingChain}
          save={pendingChain => save({...stored,pendingChain})} work={work} session={chainSession}
          persistSignature={async (wire, expires, approve) => {
            await persistWalletSignature(stored, 'chain', wire, expires, commit, async () => ({pendingChain: await approve()}));
          }}
          // Chain screens mark prompts that wait on the independent display and the board; any other
          // message (for example a saved result) ends the board step.
          prompt={(text, onBoard) => onBoard ? waitOnBoard({flow:'sign',step:'display'},text) : (setMessage(text), setBoardWait(undefined))} challenge={async () => fromBase64(await native.randomBytes(32))}
          rpc={(endpoint,packet) => native.evmRpc(endpoint,packet)} t={t} />}
      {native && stored?.verified && screen==='walletconnect' && chosenNetwork && registryCurrent && registry?.walletConnect &&
        <WalletConnectScreen scope={{board:stored.verified.boardFingerprint,account:stored.verified.address,
          generation:stored.verified.generation,chain:Number(chosenNetwork.chain),genesis:chosenNetwork.genesis}}
          key={connectSelection ?? 'new'} relay={registry.walletConnect.relay} projectId={registry.walletConnect.projectId} stored={selectedConnect}
          save={async(value,topic,stillCurrent)=>{
            if (!topic) throw new Error('WC_CHECKPOINT');
            await commit(head=>{
              if (value && stillCurrent && !stillCurrent()) throw new Error('WC_CLOSED');
              const saved=storedWalletConnects(head);
              const existing=saved.find(item=>item.checkpoint.topic===topic);
              if (value && value.checkpoint.topic!==topic) throw new Error('WC_CHECKPOINT');
              if (value && (!existing && connectSelection!==null || existing && value.checkpoint.lastId<existing.checkpoint.lastId)) throw new Error('WC_CLOSED');
              const sessions=saved.filter(item=>item.checkpoint.topic!==topic);
              if (value) sessions.push(value);
              return {...head,walletConnect:undefined,walletConnectSessions:sessions};
            });
          }}
          boards={found} scan={()=>scan()} back={()=>{setScreen(exploreBack.current==='walletconnect'?'dapp':'owner');exploreBack.current=undefined;}}
          reviewNewConnection={()=>setConnectSelection(null)} openConnections={openAllowance}
          verify={async handle=>{await verifyDappBoard(handle);externalBoard.current=handle;}}
          random={async length=>fromBase64(await native.randomBytes(length))}
          trust={async(pairing,input)=>{
            const entry=stored.evmRegistry, origin=registry.dapp?.origin;
            if (!entry?.anchor || !origin || registry.dapp?.chain!==chosenNetwork.chain) return undefined;
            const proposed=input as {proposer?:{publicKey?:string}}|undefined, peerKey=proposed?.proposer?.publicKey;
            if (typeof peerKey!=='string' || !/^[a-f0-9]{64}$/.test(peerKey)) return undefined;
            try {
              const rpc=httpsEvmRpc(entry.endpoint,(endpoint,packet)=>native.evmRpc(endpoint,packet));
              const enrollment=await rpc('nu54_walletConnectPeer',
                [{token:entry.token,chain:chosenNetwork.chain,origin,pairingTopic:pairing.topic,peerKey}]) as
                {proof:VerifiedPeer;signature:string};
              return verifyPeer(enrollment.proof,enrollment.signature,hexToBytes(entry.anchor),Math.floor(Date.now()/1000));
            } catch {return undefined;}
          }}
          allowancePolicy={dappAllowancePolicy} allowanceSubmitted={openAllowance} request={serviceExternalRequest}
          viewAllowanceConnection={origin=>viewAllowanceConnections(origin,'walletconnect')} />}
      {stored?.verified && screen==='walletconnect' && !(native && chosenNetwork && registryCurrent && registry?.walletConnect) && <>
        <Text style={styles.body}>{t('이 지갑에 WalletConnect 연결 설정이 없습니다.')}</Text>
        <Action title={t('네트워크 및 자산 설정')} variant="secondary" disabled={busy} run={()=>{exploreBack.current='evm-registry';setScreen('evm-registry');}} />
      </>}
      {stored?.verified && (screen==='walletconnect' || screen==='dapp') &&
        <Action title={t('온체인 토큰 권한')} disabled={busy} run={()=>openAllowance(screen==='dapp'||exploreBack.current==='walletconnect')} />}
      {stored?.verified && screen==='dapp' && <>
        <DappBrowserScreen scope={chosenNetwork && registryCurrent ? {board:stored.verified.boardFingerprint,account:stored.verified.address,
            generation:stored.verified.generation,chain:Number(chosenNetwork.chain),genesis:chosenNetwork.genesis} : undefined}
            boards={found} scan={()=>scan()} back={()=>setScreen('owner')}
            initialSite={exploreSite} onSiteChange={onExploreSiteChange}
            openExternal={chosenNetwork && registryCurrent && registry?.walletConnect ? ()=>openExternalConnection(undefined,true) : undefined}
            openConnections={()=>openAllowance(true)} openTravel={openExploreTravel} openMarket={openExploreMarket}
            openNetwork={()=>{exploreBack.current='evm-registry';setScreen('evm-registry');}}
            loginOrigin={registry?.dapp?.chain===chosenNetwork?.chain ? registry?.dapp?.origin : undefined}
            prepareLogin={prepareDappLogin} signLogin={signDappLogin}
            allowancePolicy={dappAllowancePolicy} prepareAllowance={prepareDappAllowance} onAllowanceSubmitted={openAllowance} onViewConnection={origin=>viewAllowanceConnections(origin,'browser')}
            prepareSpend={prepareDappSpend} signSpend={signDappSpend} submitSpend={submitDappSpend} abortLogin={()=>connection.current?.close()}
            verify={verifyDappBoard} />
      </>}
      {stored && screen === 'evm-registry' && <>
        <Text style={styles.body}>{t('등록된 지갑 서비스에서 발급한 주소와 접근 코드를 입력하세요.')}</Text>
        <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('지갑 서비스 주소')} value={registryEndpoint}
          editable={!busy} onChangeText={setRegistryEndpoint} maxLength={320} autoCorrect={false} autoCapitalize="none" />
        <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('서비스 접근 코드')} value={registryToken}
          editable={!busy} onChangeText={setRegistryToken} maxLength={64} secureTextEntry autoCorrect={false} autoCapitalize="none" />
        <Action title={t('개인 기기 찾기')} disabled={!profile || busy || !registryEndpoint || !registryToken} run={()=>scan()} />
        {found.map(item=><Action key={item.address} title={t('보드로 설정 검증')} disabled={busy || !registryEndpoint || !registryToken} run={()=>enrollEvmRegistry(item.address)} />)}
        {visibleEvmNetworks.map(network=><View key={network.chain} style={styles.device}>
          <Text style={styles.body}>{evmProfiles[network.chain].label} · {network.assets.map(asset=>asset.symbol).join(', ')}</Text>
          <Text selectable style={styles.mono}>{selectedRpc(network)}</Text>
          <Action title={t('RPC 확인')} disabled={busy} run={()=>work(async()=>{
            if (!native || !registryCurrent) throw new Error('EVM_REGISTRY');
            const rpc=httpsEvmRpc(selectedRpc(network),(endpoint,packet)=>native.evmRpc(endpoint,packet));
            await checkEvmNetwork(rpc,network.chain,network.genesis);
            setMessage(t('서명된 설정의 네트워크와 RPC가 일치합니다.'));
          },networkFailure('검증된 네트워크에 연결할 수 없습니다.'))} />
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('대체 HTTPS RPC 주소') + ' · ' + evmProfiles[network.chain].label}
            value={rpcCandidate[network.chain] ?? ''} editable={!busy} maxLength={512} autoCorrect={false} autoCapitalize="none"
            onChangeText={value=>setRpcCandidate(previous=>({...previous,[network.chain]:value}))} />
          <Action title={t('제네시스 확인 후 이 세션에서 사용')} disabled={busy || !rpcCandidate[network.chain]} run={()=>work(async()=>{
            if (!native || !registryCurrent) throw new Error('EVM_REGISTRY');
            const endpoint=rpcCandidate[network.chain].trim();
            const rpc=httpsEvmRpc(endpoint,(url,packet)=>native.evmRpc(url,packet));
            await checkEvmNetwork(rpc,network.chain,network.genesis);
            const key=rpcKey(network.chain,network.genesis);
            setRpcOverride(previous=>({...previous,[key]:endpoint}));
            setMessage(t('같은 체인 ID와 제네시스를 확인했습니다. 이 세션에서 대체 RPC를 사용합니다.'));
          },networkFailure('검증된 네트워크에 연결할 수 없습니다.'))} />
        </View>)}
        <Action title={t('등록 상태로 돌아가기')} disabled={busy} run={() => {setScreen(exploreBack.current==='evm-registry'?'dapp':'owner');exploreBack.current=undefined;}} />
      </>}
      {stored && screen === 'receive-evm' && <>
        {!registryCurrent && <Action title={t('네트워크 및 자산 설정')} disabled={busy} run={()=>setScreen('evm-registry')} />}
        {registryCurrent && chosenNetwork && <>
          <Text style={styles.body}>{t('수신 네트워크')} · {evmProfiles[chosenNetwork.chain].label}</Text>
          {visibleEvmNetworks.map(network=><Action key={network.chain} title={evmProfiles[network.chain].label} disabled={busy} run={()=>{
            setReceiveChain(network.chain);
            setReceiveAddress(''); setEvmReceiveBalance(undefined);
          }} />)}
          <Action title={t('개인 기기 찾기')} disabled={!profile || busy} run={()=>scan()} />
          {found.map(item => <Action key={item.address} title={t('보드와 수신 주소 확인')}
            disabled={busy} run={() => confirmEvmReceive(item.address)} />)}
          {!!receiveAddress && <Text selectable style={styles.mono}>{receiveAddress}</Text>}
          {!!receiveAddress && <Action title={t('입금 잔액 다시 확인')} disabled={busy} run={()=>work(async()=>{
            if (!native || !chosenAsset) throw new Error('EVM_ASSET_REQUIRED');
            setEvmReceiveBalance(undefined);
            const rpc=httpsEvmRpc(selectedRpc(chosenNetwork),(url,packet)=>native.evmRpc(url,packet));
            setEvmReceiveBalance(await readEvmReceiveBalance(rpc,chosenNetwork.chain,chosenNetwork.genesis,receiveAddress,chosenAsset.address));
          },networkFailure('확정 입금 잔액을 확인하지 못했습니다.'))} />}
          {!!receiveAddress && evmReceiveBalance && <Text selectable style={styles.mono}>
            {evmReceiveBalance.amount} · {t('최소 단위 금액')} · {t('확정 블록')} {evmReceiveBalance.block}
          </Text>}
          {!receiveAddress && <Text style={styles.body}>{t('보드 연결 전에는 이전에 확인한 주소를 읽기 전용으로 표시합니다.')}</Text>}
          {!receiveAddress && stored.verified && !stored.spendingBoard && <Text selectable style={styles.mono}>{stored.verified.address}</Text>}
        </>}
        <Action title={t('등록 상태로 돌아가기')} disabled={busy} run={() => setScreen('owner')} />
      </>}
      {stored && (screen==='transfer' || screen==='allowance') && <>
        {screen==='allowance' && <Text style={styles.body}>{t('사이트 연결을 끊어도 토큰 사용 한도는 유지됩니다.')}</Text>}
        {screen==='allowance' && <>
          <Text accessibilityRole="header" style={styles.body}>{t('사이트 연결')}</Text>
          <Text style={styles.body}>{t('허용된 연결')} {connections.length}</Text>
          {allowanceConnection && <>
            <Text style={styles.body}>{t('승인 검토를 종료했습니다. 계속하려면 사이트에서 새 요청을 보내세요.')}</Text>
            {allowanceConnection.transport==='browser' && <>
              <Text selectable style={styles.mono}>{allowanceConnection.origin}</Text>
              <Text style={styles.body}>{t('사이트 연결을 닫았습니다.')}</Text>
              <Action title={t('DApp 둘러보기')} disabled={busy} run={()=>setScreen('dapp')} />
            </>}
          </>}
          {connections.map((item,index)=>{
            const {origin}=item.checkpoint.proposal, {chain,account}=item.checkpoint.scope;
            const label=connections.filter(other=>other.checkpoint.proposal.origin===origin).length>1 ? origin+' · '+(index+1) : origin;
            return <View key={item.checkpoint.topic} style={styles.device}>
              <Text selectable style={styles.mono}>{origin}</Text>
              <Text style={styles.body}>{t(item.checkpoint.proposal.verified ? '등록된 출처' : '출처 미확인 · 계정 조회만')}</Text>
              <Text style={styles.body}>{evmProfiles[String(chain) as EvmChain].label}</Text>
              <Text selectable style={styles.mono}>{account}</Text>
              <Action title={t('세션 해제')+' · '+label} disabled={busy} run={()=>disconnectConnections([item])} />
              <Action title={connections.length===1 ? t('외부 DApp 연결 관리') : t('외부 DApp 연결 관리')+' · '+label}
                disabled={busy} run={()=>openExternalConnection(item)} />
            </View>;
          })}
          {!!savedConnections.length && <Action title={t('전체 연결 해제')} disabled={busy} run={confirmDisconnectAll} />}
          {registryCurrent && registry?.walletConnect && <Action title={t('새 외부 DApp 연결')} disabled={busy} run={()=>openExternalConnection()} />}
          <Text accessibilityRole="header" style={styles.body}>{t('온체인 토큰 권한')}</Text>
        </>}
        {!stored.pendingEvm && !registryCurrent && <Action title={t('네트워크 및 자산 설정')} disabled={busy} run={()=>setScreen('evm-registry')} />}
        {!stored.pendingEvm && registryCurrent && chosenNetwork && chosenAsset && <>
          <Text style={styles.body}>{evmProfiles[chosenNetwork.chain].label} · {chosenAsset.symbol}</Text>
          <Text selectable style={styles.mono}>{stored.verified?.address}</Text>
          {visibleEvmNetworks.map(network=><Action key={network.chain} title={evmProfiles[network.chain].label} disabled={busy || network.chain===chosenNetwork.chain} run={()=>{
            setReceiveChain(network.chain);setEvmAsset('');setEvmRecipient('');setEvmAmount('');setTransfer(undefined);setAllowanceRead(undefined);
          }} />)}
          {chosenNetwork.assets.map(asset=><Action key={asset.address} title={asset.symbol} disabled={busy || asset.address===chosenAsset.address}
            run={()=>{setEvmAsset(asset.address);setEvmAmount('');setTransfer(undefined);setAllowanceRead(undefined);}} />)}
          {screen==='transfer' ? <>
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('수취인')} editable={!busy} value={evmRecipient}
            onChangeText={value=>{setEvmRecipient(value);setTransfer(undefined);}} maxLength={42} autoCorrect={false} autoCapitalize="none" />
          {stored.evmRecipients?.filter(item=>item.genesis===chosenNetwork.genesis).map(item=><Action key={item.address}
            title={item.name} disabled={busy} run={()=>{setEvmRecipient(item.address);setTransfer(undefined);}} />)}
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('연락처 이름')} editable={!busy} value={contactName}
            onChangeText={setContactName} maxLength={32} autoCorrect={false} />
          <Action title={t('이 네트워크 주소록에 저장')} disabled={busy || !contactName || !evmRecipient} run={()=>work(async()=>{
            const address=normalizeEvmAddress(evmRecipient.trim()), name=contactName.trim();
            if (!name) throw new Error('EVM_RECIPIENT');
            const previous=(stored.evmRecipients??[]).filter(item=>item.genesis!==chosenNetwork.genesis || item.address!==address);
            if (previous.length>=12) throw new Error('CONTACTS_FULL');
            await save({...stored,evmRecipients:[...previous,{name,genesis:chosenNetwork.genesis,address}]});
            setContactName('');
          },t('수취인 주소를 저장할 수 없습니다.'))} />
          <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('전송 금액')} editable={!busy} value={evmAmount}
            onChangeText={value=>{setEvmAmount(value);setTransfer(undefined);}} maxLength={100} keyboardType="decimal-pad" />
          <Action title={t('전송 내용 검토')} disabled={!evmRecipient || !evmAmount || busy} run={()=>quoteEvmTransfer()} />
          </> : <>
            {chosenAsset.address==='0x'+'0'.repeat(40) ? <Text style={styles.body}>{t('사용 한도를 조회할 토큰을 선택하세요.')}</Text> : <>
              <Text selectable style={styles.mono}>Chain ID {chosenNetwork.chain} · {chosenNetwork.genesis}</Text>
              <Text selectable style={styles.mono}>{t('토큰 계약')} {chosenAsset.address}</Text>
              <TextInput placeholderTextColor={color.mute} style={styles.input} accessibilityLabel={t('권한 대상 주소')} editable={!busy && !transfer}
                value={allowanceSpender} onChangeText={value=>{setAllowanceSpender(value);setAllowanceRead(undefined);}}
                maxLength={42} autoCorrect={false} autoCapitalize="none" />
              {!transfer && <Action title={t('확정 한도 조회')} disabled={!activeAllowanceScope || busy} run={readAllowance} />}
              {!transfer && allowance?.value!==undefined && <Text style={styles.body}>{t('현재 확정 한도')} {allowance.value.toString()} {t('최소 단위 금액')} · {formatEvmTokenAmount(allowance.value,chosenAsset)}</Text>}
              {!transfer && (allowance?.value ?? 0n)>0n && <Action title={t('토큰 사용 권한 철회 검토')} disabled={busy} run={()=>quoteEvmTransfer(true)} />}
            </>}
          </>}
          {transfer && transfer.revoke===(screen==='allowance') && <>
            <Text selectable style={styles.mono}>{t(transfer.revoke ? '권한 대상' : '수취인')} {transfer.recipient}</Text>
            <Text style={styles.body}>{t(transfer.revoke ? '변경 후 사용 한도' : '전송 금액')} {transfer.revoke ? ((allowance?.value?.toString() ?? '—')+' → 0 '+t('최소 단위 금액')) : evmAmount} {chosenAsset.symbol}</Text>
            <Text selectable style={styles.mono}>Chain ID {transfer.chain} · Nonce {transfer.nonce.toString()}</Text>
            <Text selectable style={styles.mono}>{transfer.genesis}</Text>
            <Text style={styles.body}>{t('실행 수수료 상한')} {transfer.executionMaximum.toString()} wei</Text>
            <Text style={styles.body}>{t('수수료 견적 (wei)')} {transfer.maximum.toString()}</Text>
            {transfer.l1Fee > 0n && <Text style={styles.body}>Base L1 + operator {transfer.l1Fee.toString()} wei</Text>}
            <Action title={t('거절하고 초안으로')} disabled={busy} run={()=>{
              setTransfer(undefined); setTransferWire('');
              setMessage(t('이 원문을 거절했습니다. 서명 0건 · 제출 0건'));
            }} />
            <Action title={t('개인 기기 찾기')} disabled={!profile || busy} run={()=>scan()} />
            {found.map(item => <Action key={item.address} title={t(transfer.revoke ? '기기에 토큰 권한 철회 승인 요청' : '기기에 전송 승인 요청')} disabled={busy}
              run={() => signTransfer(item.address)} />)}
          </>}
        </>}
        {screen==='allowance' && stored.pendingEvm && !pendingAllowance &&
          <Action title={t('기존 거래 상태 확인 필요')} disabled={busy} run={()=>setScreen('transfer')} />}
        {stored.pendingEvm && (screen==='transfer' || !!pendingAllowance) && <>
          <Text style={styles.body}>{evmProfiles[stored.pendingEvm.chain].label}</Text>
          {screen==='allowance' && pendingAllowance && <>
            <Text selectable style={styles.mono}>{t('계정')} {stored.pendingEvm.sender}</Text>
            <Text selectable style={styles.mono}>{t('토큰 계약')} {pendingAllowance.token}</Text>
            <Text selectable style={styles.mono}>{t('권한 대상')} {pendingAllowance.spender}</Text>
            <Text style={styles.body}>Chain ID {stored.pendingEvm.chain}</Text>
            <Text style={styles.body}>{t('요청 한도')} {pendingAllowance.amount.toString()} {t('최소 단위 금액')}</Text>
            {allowance?.value!==undefined && allowance.result==='finalized' && allowance.activeHash===stored.pendingEvm.hash
              ? <Text style={styles.body}>{t('현재 확정 한도')} {allowance.value.toString()} {t('최소 단위 금액')}</Text>
              : <Text style={styles.body}>{t('같은 해시를 조회해 확정 한도를 확인하세요.')}</Text>}
            {stored.pendingEvm.submitted && allowance?.result!=='reverted' && allowance?.result!=='finalized' &&
              <Text style={styles.body}>{t(pendingApproval ? '권한 부여 거래 확정 대기' : '철회 거래 확정 대기')}</Text>}
            {allowance?.result==='reverted' && <Text style={styles.body}>{t(pendingApproval ? '권한 부여 거래가 실패했습니다.' : '철회 거래가 실패했습니다.')}</Text>}
          </>}
          <Text selectable style={styles.mono}>{stored.pendingEvm.hash}</Text>
          {screen==='transfer' && pendingEvmReview && <>
            <Text selectable style={styles.mono}>{t('수취인')} {pendingEvmReview.recipient}</Text>
            <Text style={styles.body}>Chain ID {stored.pendingEvm.chain} · Nonce {pendingEvmReview.nonce.toString()}</Text>
            <Text selectable style={styles.mono}>{pendingEvmReview.amount.toString()} · {t('최소 단위 금액')} · {pendingEvmReview.asset}</Text>
            <Text style={styles.body}>{t('실행 수수료 상한')} {pendingEvmReview.executionMaximum.toString()} wei</Text>
          </>}
          <Text style={styles.body}>{t(stored.pendingEvm.submitted ? '기존 거래 상태 확인 필요' : '서명됨 · 전송 전')}</Text>
          {!stored.pendingEvm.rpc && stored.pendingEvm.genesis && <TextInput placeholderTextColor={color.mute} style={styles.input}
            accessibilityLabel={t('검증된 HTTPS RPC 주소')} value={rpcEndpoint}
            onChangeText={setRpcEndpoint} maxLength={512} autoCorrect={false} autoCapitalize="none" />}
          {stored.pendingEvm.genesis
            ? <Text selectable style={styles.mono}>{stored.pendingEvm.genesis}</Text>
            : <Text style={styles.body}>{t('이전 거래의 네트워크 근거가 없습니다. 위 해시를 보관해 원래 네트워크에서 대사하세요.')}</Text>}
          {stored.pendingEvm.genesis && !stored.pendingEvm.submitted && !pendingApproval && <Action title={t('이 네트워크에 전송')} disabled={busy || !(stored.pendingEvm.rpc || rpcEndpoint)}
            run={() => currentTransfer('submit')} />}
          <Text style={styles.body}>{language==='ko' ? '서명된 거래는 앱의 기한이 지나도 전송될 수 있습니다. 휴대전화 조회는 보드 정산이 아니며, 재시작 후에도 원문과 미결을 보관합니다. 기존 기기의 이전 정책 이관은 모든 과거 서명 기록을 증명할 수 없어 보류 중입니다.' : 'A signed transaction may still be broadcast after the app deadline. A phone lookup is not board reconciliation; the original and pending request remain after restart. Legacy policy migration is on hold without a complete record of prior signatures.'}</Text>
          <Text style={styles.body}>{language==='ko' ? '내보낸 서명은 다른 사람이 전송할 수 있으므로 신뢰하는 곳에만 공유하세요.' : 'Anyone holding the exported signature may broadcast it. Share only with a trusted recipient.'}</Text>
          <Action title={language==='ko'?'저장된 거래 내보내기':'Export saved transaction'} disabled={busy} run={()=>work(async()=>{
            const pending=currentStorage.current?.pendingEvm;if(!pending)throw new Error('EVM_PENDING_MISSING');
            await Share.share({message:JSON.stringify({type:'nu54-saved-evm-v1',chain:pending.chain,genesis:pending.genesis,hash:pending.hash,sender:pending.sender,raw:pending.raw,evidence:pending.evidence},null,2)});
          },language==='ko'?'거래를 내보내지 못했습니다.':'Could not export the transaction.')} />
          {stored.pendingEvm.genesis && <Action title={t(screen==='allowance' ? (pendingApproval ? '같은 권한 부여 거래 조회' : '같은 철회 거래 조회') : '전송 상태 재조회')} disabled={busy || !(stored.pendingEvm.rpc || rpcEndpoint)} run={() => currentTransfer('query')} />}
        </>}
        <Action title={t('등록 상태로 돌아가기')} disabled={busy} run={screen==='allowance' ? closeAllowance : () => setScreen('owner')} />
      </>}
      {stored&&(maintenanceScreen||screen==='rental-maintenance'||screen==='rental-original-payment')&&<WalletMaintenance
        rentalOriginalDetail={screen==='rental-original-payment'} navigateRentalOriginal={navigateRentalOriginal}
        language={language} scope={maintenanceScreen?maintenanceScope(stored):{mode:'rental'}}
        active={homeActive} signing={!!stored.signing} unsettled={!!(stored.pendingEvm||stored.pendingChain)}
        networkBusy={busy} devices={found} scan={()=>scan()}
        read={handle=>maintenanceRequest(handle,session=>session.maintenanceStatus())}
        runningFirmware={(handle,expected)=>maintenanceRequest(handle,session=>session.runningFirmware(expected))}
        candidateTrust={candidateTrustForInstallation(firmwareCandidateDeployment,
          stored.installation?.state==='consumed'?stored.installation.claim:undefined)} pickCandidate={native?()=>native.pickInstallationFile():undefined}
        shareCandidateContext={async context=>{await Share.share({message:context});}}
        inputPower={(handle,expected)=>maintenanceRequest(handle,session=>session.firmwareInputPower(expected))}
        tools={(handle,expected)=>maintenanceRequest(handle,session=>session.deviceTool(expected,'read'))}
        find={(handle,expected,start)=>maintenanceRequest(handle,session=>session.deviceTool(expected,start?'find':'cancel'))}
        speak={text=>read(text)}
        campaign={(handle,request,expected)=>maintenanceRequest(handle,session=>session.firmwareProof(request,expected))}
        backup={()=>setScreen('backup')} orders={orders}
        rental={Platform.OS==='android'?rental:undefined}
        backLabel={screen==='policy-device'?(language==='ko'?'보호 정책으로 돌아가기':'Return to protection policy'):screen==='settings-device'?(language==='ko'?'설정으로 돌아가기':'Return to Settings'):undefined}
        back={()=>screen==='policy-device'?returnToPolicy():setScreen(screen==='firmware'?'owner':screen==='settings-device'?'settings':'profiles')}/>}
      {screen==='recovery-options' && <>
        <Text style={styles.body}>{language==='ko'?'보유한 기기와 복구 자료에 맞는 방법을 고르세요.':'Choose the method that matches the device and recovery material you still have.'}</Text>
        <Action testID="w06-choice-phone" title={language==='ko'?'잃어버린 휴대전화 교체':'Replace a lost phone'} run={()=>chooseRecovery('replace-phone')} />
        <Text style={styles.body}>{language==='ko'?'원래 보드와 보드 PIN, 등록한 독립 복구 장치가 필요합니다.':'You need the original board, its PIN, and the registered independent recovery device.'}</Text>
        <Action testID="w06-choice-board" title={language==='ko'?'분실한 보드 복원':'Restore a lost board'} run={()=>chooseRecovery('recovery')} />
        <Text style={styles.body}>{language==='ko'?'새 보드와 검증된 백업, 독립 검증자 지문이 필요합니다.':'You need a replacement board, a verified backup, and the independent verifier fingerprint.'}</Text>
        <Action testID="w06-choice-cloud" title={language==='ko'?'클라우드 휴대전화 복구':'Cloud phone recovery'} run={()=>chooseRecovery('recovery-cloud-info')} />
        <Text style={styles.body}>{language==='ko'?'기존 클라우드 지갑 계정으로 별도로 인증합니다.':'Authenticate separately with your existing cloud wallet account.'}</Text>
        <Action testID="w06-recovery-cancel" title={language==='ko'?'취소':'Cancel'} run={leaveRecoveryOptions} />
      </>}
      {screen==='recovery-cloud-info' && <>
        <Text style={styles.body}>{cloudRecoveryAvailable
          ? (language==='ko'?'기존 클라우드 계정으로 로그인해 복구 자격을 확인하세요. 복구 접수는 인증 뒤 별도로 선택합니다.':'Sign in to your existing cloud account to check recovery eligibility. Starting recovery is a separate step after sign-in.')
          : (language==='ko'?'이 설치에서는 클라우드 휴대전화 복구를 사용할 수 없습니다. 다른 복구 방법을 선택할 수 있습니다.':'Cloud phone recovery is unavailable on this installation. You can choose another recovery method.')}</Text>
        {cloudRecoveryAvailable && <Action title={language==='ko'?'기존 클라우드 계정 인증':'Authenticate existing cloud account'}
          run={()=>setScreen('recovery-cloud')} />}
        <Action testID="w06-recovery-back" title={backToRecovery} run={returnToRecoveryOptions} />
      </>}
      {stored && screen === 'backup' && <>
        <Text style={styles.body}>{t('등록된 원본 기기와 독립 복구 장치를 준비하세요. 복원 시험을 마치고 원본 기기에서 확인해야 백업 상태가 완료됩니다.')}</Text>
        {backup && <BoardStepPanel wait={{flow:'backup',step:'hold'}} t={t} />}
        {!stored.verified && (backup || backupBlocked) && <>
          <Text accessibilityRole="header" accessibilityLiveRegion="polite" style={styles.title}>{t('입금 안내가 잠겨 있습니다')}</Text>
          <Text style={styles.body}>{t('기존 활성 슬롯 — 보존')}</Text>
          <Text style={styles.body}>{t('대조 결과 — 미확인')}</Text>
          <Text style={styles.body}>{t('별도 보드에서 주소가 다르면 임시 슬롯을 폐기하고 보관한 기준 주소와 파생 경로를 다시 대조하세요.')}</Text>
        </>}
        {backupBlocked && <>
          <Text style={styles.body}>{t('이 기기에서 독립 백업 요청을 열 수 없습니다. 등록된 원본 기기와 별도 복구 장치를 확인하세요.')}</Text>
          <Action title={t('다시 확인')} disabled={busy} run={()=>setBackupBlocked(false)} />
        </>}
        {!backup && !backupBlocked && <>
          <Action title={t('개인 기기 찾기')} disabled={busy || !native || !profile} run={()=>scan()} />
          {found.map(item => <View key={item.address} style={styles.device}>
            <Text style={styles.body}>{item.name || t('이름이 표시되지 않는 기기')}</Text>
            <Action title={t('원본 기기에 백업 요청')} disabled={busy} run={() => startBackup(item.address)} />
          </View>)}
        </>}
        {backup && <>
          <Text style={styles.body}>{t('백업 요청 번호')}</Text>
          <Text selectable style={styles.mono}>{backup.requestId}</Text>
          <Action title={t('원본 기기의 백업 상태 확인')} disabled={busy} run={checkBackup} />
        </>}
        <Action title={t('등록 상태로 돌아가기')} disabled={busy} run={() => {endBackup();setScreen('owner');}} />
      </>}
      {stored && screen === 'replace-phone' && <>
        <Text style={styles.body}>{t('원래 보드, 등록한 독립 복구 장치, 보드 PIN을 준비하세요. 보드에서 SW3을 길게 눌러 교체를 시작하세요.')}</Text>
        {!lostPhone && !boardWait && <BoardStepPanel wait={{flow:'replace',step:'recovery'}} t={t} />}
        {!lostPhone && <>
          {routeEnrollmentForm}
          <Action title={t('개인 기기 찾기')} disabled={!native || !routeReady || busy} run={()=>scan(false,!!recoveryOrigin)} />
          {found.map(item => <View key={item.address} style={styles.device}>
            <Text style={styles.body}>{item.name || t('이름이 표시되지 않는 기기')}</Text>
            {stored.phoneReplacement
              ? <Action title={t('교체 후 보드 상태 확인')} disabled={busy} run={() => verifyLostPhone(item.address)} />
              : <Action title={t('분실한 휴대전화 교체 요청')} disabled={busy} run={() => startLostPhone(item.address)} />}

          </View>)}
        </>}
        {lostPhone && <>
          <Text style={styles.body}>{t('독립 복구 장치에 전달할 요청')}</Text>
          <Text selectable style={styles.mono}>{lostPhone.previous.challenge}</Text>
          <Text style={styles.body}>{t('기존 지갑 주소')}</Text>
          <Text selectable style={styles.mono}>{lostPhone.previous.address}</Text>
          {!lostPhone.submitted && <Action title={t('새 휴대전화 키 증명')} disabled={busy} run={submitLostPhone} />}
          {lostPhone.submitted && <Action title={t('교체 후 보드 상태 확인')} disabled={busy} run={() => verifyLostPhone(lostPhone.handle)} />}
        </>}
        <Action testID={recoveryOrigin?'w06-recovery-back':undefined} title={recoveryOrigin?backToRecovery:t('등록 상태로 돌아가기')}
          disabled={busy} run={recoveryOrigin ? returnToRecoveryOptions : ()=>{closeLostPhone();setScreen('owner');}} />
      </>}
      {stored && screen === 'recovery' && <>
        {recoveryOrigin && <Text style={styles.body}>{language==='ko'?'새 보드와 검증된 백업을 준비하고 원본·교체 기기·검증자의 지문을 대조하세요.':'Prepare the replacement board and verified backup, then compare the original board, replacement board and verifier fingerprints.'}</Text>}
        <Text style={styles.body}>{t('새 휴대전화 등록서와 복구용 기기를 준비하세요. 복구 기기가 기존 지갑 주소와 네트워크를 확인하기 전까지 결제 승인은 잠겨 있습니다.')}</Text>
        <Text style={styles.body} accessible={false}>{t('독립 복구 검증자 등록서')}</Text>
        <TextInput placeholderTextColor={color.mute} testID="w06-trust-document" style={[styles.input,styles.recoveryDocument]} accessibilityLabel={t('독립 복구 검증자 등록서')}
          value={trustDoc} onChangeText={value=>{if(recoveryTarget)endRecovery();setTrustDoc(value);}} maxLength={2048} multiline autoCorrect={false} autoCapitalize="none" editable={!busy && !restore} />
        {([['original','원래 기기 공개키 지문'],['new','새 기기 공개키 지문'],['verifier','독립 복구 장치 공개키 지문']] as const).map(([id,label],i) =>
          <View key={id}>
            <Text style={styles.body} accessible={false}>{t(label)}</Text>
            <TextInput placeholderTextColor={color.mute} testID={`w06-${id}-fingerprint`} style={styles.input} accessibilityLabel={t(label)} value={pins[i]}
              onChangeText={value=>{if(recoveryTarget)endRecovery();setPins(previous=>previous.map((pinValue,index)=>i===index?value:pinValue));}}
              maxLength={64} returnKeyType="done" submitBehavior="blurAndSubmit" editable={!busy && !restore} autoCorrect={false} autoCapitalize="none" />
          </View>)}
        {!restore && <>
          {routeEnrollmentForm}
          {!recoveryOrigin && <Action title={t('다른 복구용 기기 등록')} disabled={busy} run={() => {
            endRecovery();
            setProfile(undefined);
            setStatus(undefined);
            setFound([]);
            setScreen('owner');
          }} />}
          <Action title={t('새 기기 찾기')} disabled={!native || !routeReady || busy}
            run={()=>{if(!recoveryOrigin || recoveryTarget)endRecovery();scan(false,!!recoveryOrigin);}} />
          {found.map((item,index) => <View key={item.address} style={styles.device}>
            <Text style={styles.body}>{item.name || t('이름이 표시되지 않는 기기')}</Text>
            <Action testID={`w06-recover-candidate-${index+1}`}
              title={t('등록 대상 확인')+' · '+(language==='ko'?'검색 후보':'Candidate')+' '+(index+1)}
              disabled={busy} run={() => verifyRecoveryTarget(item.address)} />
          </View>)}
          {recoveryTarget&&<>
            <Text accessibilityRole="header">{t('인증된 복구 대상')}</Text>
            <Text style={styles.body}>{t('등록서와 일치하는 기기 식별자')}</Text>
            <Text style={styles.mono} selectable>{recoveryTarget.device}</Text>
            <Text style={styles.body}>{t('등록서와 일치하는 공개키 지문')}</Text>
            <Text style={styles.mono} selectable>{recoveryBoard?.boardSPKI}</Text>
            <Action testID="w06-authenticated-restore" title={t('확인한 기기에서 복구 요청')} disabled={busy} run={startRecovery} />
          </>}
        </>}
        {restore && <BoardStepPanel wait={{flow:'backup',step:'hold'}} t={t} />}
        {restore && <>
          <Text style={styles.body}>{t('복구 요청 번호')}</Text><Text selectable style={styles.mono}>{restore.requestId}</Text>
          <Action title={t('복구 증거 확인')} disabled={busy} run={checkRecovery} />
          <Action title={t('복구 종료')} disabled={busy} run={() => {
            endRecovery();
            setMessage('복구 연결을 종료했습니다. 상태를 다시 확인하세요.');
          }} />
        </>}
        <Action title={t('안내 읽기')} disabled={!native || busy} run={() => work(() => read('새 휴대전화 등록서와 복구용 기기를 준비하세요. 복구 증거 확인 전에는 결제 승인이 잠겨 있습니다.'), '기기에서 읽어주기를 사용할 수 없습니다.')} />
        <Action testID={recoveryOrigin?'w06-recovery-back':undefined} title={recoveryOrigin?backToRecovery:t('등록 상태로 돌아가기')}
          disabled={busy} run={recoveryOrigin ? returnToRecoveryOptions : ()=>{endRecovery();setScreen('owner');}} />
      </>}
      {(stored || !tabbed) && (screen==='profiles'||screen===(tabbed?'home':'owner')) && !!services.length && <Section title={t(tabbed?'바로 가기':'서비스')}>
        {services.map(service => <Action key={service.label} title={t(service.label)} run={service.run} disabled={busy} variant="row" />)}
      </Section>}
    </ScrollView>
  );
  if (screen === 'demo' && demoAllowed) return <DemoWalletHome t={t} close={() => setScreen('owner')} />;
  if (homeTabs && !tabbed) return <View style={styles.tabbed}>{content}
    <TabBar current="wallet" disabled={busy} tabs={[
      {key:'wallet',label:t('지갑'),run:()=>setScreen('owner')},
      {key:'pay',label:t('결제'),run:orders},
      {key:'explore',label:t('둘러보기'),run:()=>{dappSigned.current=undefined;setScreen('dapp');}},
      {key:'settings',label:language === 'ko' ? '설정' : 'Settings',run:openSettings},
    ]} />
  </View>;
  if(screen==='recovery'&&!restore)return <View style={styles.recoveryScreen}>
    <Action testID="w06-dismiss-keyboard" title={t('키보드 닫기')} run={Keyboard.dismiss} />
    {content}
  </View>;
  return content;
}

const styles = StyleSheet.create({
  ownerDisplayQr:{width:220,height:220},
  recoveryScreen:{flex:1,gap:8},
  recoveryDocument:{minHeight:80,maxHeight:160,textAlignVertical:'top'},
  content: {paddingBottom: 24, gap: 12},
  tabbed: {flex: 1},
  trialTitle: {fontFamily: font, fontSize: 20, fontWeight: '800', color: color.mint},
  trialBody: {fontFamily: font, fontSize: 15, lineHeight: 22, color: color.onGreen},
  languageRow: {flexDirection: 'row', justifyContent: 'flex-end'},
  pair: {flexDirection: 'row', gap: 10},
  pairItem: {flex: 1},
  sectionTitle: {fontFamily: font, fontSize: 20, fontWeight: '700', color: color.ink},
  title: {fontFamily: font, fontSize: 28, fontWeight: '800', letterSpacing: -0.8, color: color.ink},
  body: {fontFamily: font, fontSize: 16, lineHeight: 24, color: color.ink},
  mono: {fontFamily: 'monospace', color: color.ink, fontSize: 13, backgroundColor: color.sheet, borderRadius: 8, padding: 10},
  input: {borderWidth: 1, borderColor: '#8fa497', borderRadius: 12, minHeight: 50, padding: 12, color: color.ink, backgroundColor: color.sheet},
  device: {borderWidth: 1, borderColor: color.line, borderRadius: 18, padding: 16, gap: 10, backgroundColor: color.sheet},
});
