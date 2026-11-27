import { DeploymentConsole } from "./DeploymentConsole";
import { OperatorHome } from "./OperatorHome";
import { BranchPicker, useAuthorizedBranches } from "./BranchPicker";
import { tokenMoney } from "./money";
import {
  assetToken,
  humanAmount,
  priceProblem,
  shownTotal,
  type CatalogAsset,
} from "./prices";
import { localScheduleValue, scheduleCandidates } from "./localSchedule";
import type { CommerceMenuMetadata } from "@nu54/protocol";
import { CatalogCategories, CategoryReview, type CategoryDraft } from "./CatalogCategories";
import { MenuDeclarations, menuDeclarationSummary } from "./MenuDeclarations";
import { OptionReview, ProductEditor, ProductList } from "./MenuOptions";
import { PhotoField, PhotoReview, photoLimit, type PhotoInfo } from "./MenuPhotos";
import { StoreState, type Product as StoreProduct } from "./StoreState";
import { TaxField, TaxReview } from "./MenuTax";
import { emptyGroup, emptyProduct, menuLines, productProblems, publishedProducts, rowLimit, savedProducts, type Product, type SavedOptions } from "./optionGroups";
import { useRef, useState, type FormEvent } from "react";
import "./operations.css";
import { Experience } from "./Experience";
import { Fleet, type FleetScope } from "./Fleet";
import { Payout } from "./Payout";
import { Finance } from "./Finance";
import { EscrowClaims } from "./EscrowClaims";
import { EscrowDisputes } from "./EscrowDisputes";

type CatalogRow = {
  sku: string;
  option: string;
  label: string;
  asset: string;
  price: string;
  available: number;
  optionAvailable?: number | null;
};
type Hours = { opens: string; closes: string; zone: string };
type Catalog = Partial<CategoryDraft> & {
  version: number;
  timezone: string;
  expiresAt: string | null;
  rows: CatalogRow[];
  hours?: Hours | null;
  options?: SavedOptions[];
  breakdown?: Array<{
    sku: string;
    option: string;
    displayPrice: string;
    tax: string;
    discount: string;
    required?: boolean;
    metadata?: CommerceMenuMetadata;
  }>;
};
type Asset = CatalogAsset & {
  address: string;
  chainId: number;
};
type Site = {
  branch: string;
  receiptScope: string;
  catalog: Catalog;
  asset?: Asset;
  photos?: Array<PhotoInfo & { sku: string; thumb: string }>;
};
const sitePhotos = (site: Site): Record<string, PhotoInfo> =>
  Object.fromEntries((site.photos ?? []).map(({ sku, photo, bytes, confirmedAt }) => [sku, { photo, bytes, confirmedAt }]));
type Draft = Pick<
  CatalogRow,
  "sku" | "option" | "label" | "asset" | "price"
> & {
  tax: string;
  discount: string;
  required: boolean;
  metadata?: CommerceMenuMetadata;
};
type Pending = {
  branch: string;
  scope: string;
  operation: "publish" | "stock" | "option-stock";
  body: Record<string, unknown>;
};
const legacyPendingKey = "nu54-operator-catalog-pending-v1";
// Browser storage already separates issuer origins. The server-derived scope
// separates authenticated certificate identities and branches within an origin.
const pendingKey = (scope: string) =>
  `nu54-operator-catalog-pending-v2/${scope}`;
const uuid =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
function validatePending(raw: string, site: Site): Pending {
  const saved = JSON.parse(raw) as Pending;
  if (
    saved?.branch !== site.branch ||
    saved.scope !== site.receiptScope ||
    !["publish", "stock", "option-stock"].includes(saved.operation) ||
    !uuid.test(String(saved.body?.requestKey))
  )
    throw new Error("STORAGE");
  return saved;
}

function scopedPending(site: Site): Pending | undefined {
  const key = pendingKey(site.receiptScope);
  const raw = localStorage.getItem(key);
  if (raw) return validatePending(raw, site);

  const legacy = localStorage.getItem(legacyPendingKey);
  if (!legacy) return undefined;
  let candidate: Partial<Pending> | null;
  try {
    candidate = JSON.parse(legacy) as Partial<Pending> | null;
  } catch {
    // An unattributable legacy record cannot block an authenticated branch.
    return undefined;
  }
  if (
    candidate?.branch !== site.branch ||
    candidate.scope !== site.receiptScope
  )
    return undefined;
  const saved = validatePending(legacy, site);
  localStorage.setItem(key, JSON.stringify(saved));
  if (localStorage.getItem(legacyPendingKey) === legacy)
    localStorage.removeItem(legacyPendingKey);
  return saved;
}

const fresh = (): Draft => ({
  sku: "",
  option: "",
  label: "",
  asset: "",
  price: "",
  tax: "",
  discount: "",
  required: false,
});
const names: Record<Exclude<keyof Draft, "required" | "metadata">, string> = {
  sku: "상품 코드",
  option: "옵션 코드",
  label: "표시 이름",
  asset: "결제 자산",
  price: "기본 가격",
  tax: "세금",
  discount: "할인",
};

function localDeadline(value: string, zone: string) {
  if (!Number.isFinite(Date.parse(value))) return "확인 필요";
  try {
    return (
      new Intl.DateTimeFormat("ko-KR", {
        dateStyle: "medium",
        timeStyle: "short",
        timeZone: zone,
      }).format(new Date(value)) + ` · ${zone}`
    );
  } catch {
    return "시간대 확인 필요";
  }
}

function catalogChanges(before: Draft[], after: Draft[]) {
  const key = (row: Draft) => `${row.sku}/${row.option}`;
  const current = new Map(before.map((row) => [key(row), row]));
  const next = new Map(after.map((row) => [key(row), row]));
  return [...new Set([...current.keys(), ...next.keys()])].flatMap((id) => {
    const previous = current.get(id);
    const updated = next.get(id);
    return previous?.price === updated?.price &&
      previous?.label === updated?.label &&
      previous?.tax === updated?.tax &&
      previous?.discount === updated?.discount &&
      previous?.required === updated?.required &&
      previous?.asset === updated?.asset &&
      menuDeclarationSummary(previous?.metadata) ===
        menuDeclarationSummary(updated?.metadata)
      ? []
      : [{ id, previous, updated }];
  });
}

/** The code of a combination is made up by this page, so its name is shown instead. */
function optionName(site: Site, row: { sku: string; option: string }) {
  if (!site.catalog.options?.some(({ sku }) => sku === row.sku)) return row.option;
  return site.catalog.breakdown?.find(({ sku, option }) => sku === row.sku && option === row.option)?.metadata?.optionLabels?.ko ?? row.option;
}

const defaultHours: Hours = {
  opens: "10:00",
  closes: "19:00",
  zone: "Asia/Seoul",
};
function priceDescription(row: Draft, asset?: Asset) {
  return `${row.label} ${asset ? shownTotal(row, asset) : row.price} · 기본 ${row.price}, 세금 ${
    row.tax || "0"
  }, 할인 ${row.discount || "0"}${row.required ? " · 선택 필수" : ""}`;
}
/** Rows typed one by one. The combinations of a product with groups are made from its groups. */
function editableRows(site: Site): Draft[] {
  const grouped = new Set(site.catalog.options?.map(({ sku }) => sku));
  return publishedRows(site).filter(({ sku }) => !grouped.has(sku));
}
function publishedRows(site: Site): Draft[] {
  const breakdown = new Map(
    site.catalog.breakdown?.map((row) => [row.sku + "/" + row.option, row])
  );
  const format = (amount: string, asset: string) =>
    site.asset?.address === asset
      ? humanAmount(amount, site.asset.decimals)
      : amount;
  return site.catalog.rows.map(({ sku, option, label, asset, price }) => {
    const terms = breakdown.get(sku + "/" + option);
    return {
      sku,
      option,
      label,
      asset,
      price: format(terms?.displayPrice ?? price, asset),
      tax: format(terms?.tax ?? "0", asset),
      discount: format(terms?.discount ?? "0", asset),
      required: terms?.required ?? option !== "standard",
      metadata: terms?.metadata,
    };
  });
}

async function exchange(path: string, body?: Record<string, unknown>) {
  const reply = await fetch(
    path,
    body
      ? {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        }
      : { credentials: "same-origin" }
  );
  if (!reply.ok) {
    if (reply.status === 404) throw new Error("DENIED");
    if (reply.status === 409) throw new Error("CONFLICT");
    if (reply.status === 400) throw new Error("INVALID");
    throw new Error("UNKNOWN");
  }
  return reply.json();
}
/** One entry per product of the published menu, for marking sold out. */
function storeProducts(rows: Site["catalog"]["rows"]): StoreProduct[] {
  const products = new Map<string, StoreProduct>();
  for (const row of rows) {
    const known = products.get(row.sku);
    if (known) known.rows += 1;
    else products.set(row.sku, { sku: row.sku, label: row.label, rows: 1, available: Number(row.available) });
  }
  return [...products.values()];
}
const endpoint = (branch: string) =>
  `/v2/operations/catalog/${encodeURIComponent(branch)}`;

export function App() {
  const [page, setPage] = useState(() =>
    new URLSearchParams(window.location.search).get("view") === "v1"
      ? "v1"
      : "home"
  );
  const [assignedBranch, setAssignedBranch] = useState("");
  const [rentalScope, setRentalScope] = useState<FleetScope>();
  const choose = (page: string, branch = "") => {
    setRentalScope(undefined);
    setAssignedBranch(branch);
    setPage(page);
  };
  const sections = [
    { id: "home", label: "지점 운영 홈" },
    { id: "catalog", label: "카탈로그" },
    { id: "finance", label: "정산과 이행" },
    { id: "claims", label: "대금 정산" },
    { id: "disputes", label: "이의 처리" },
    { id: "experience", label: "예약과 구독" },
    { id: "payout", label: "지급 주소" },
    { id: "rental", label: "대여 기기" },
    { id: "v2", label: "v2 지급 증거" },
    { id: "v1", label: "v1 적립 인출" },
    { id: "fota", label: "펌웨어 캠페인" },
  ];
  return (
    <main>
      <nav aria-label="운영 업무">
        {sections.map(({ id, label }) => (
          <button
            key={id}
            aria-current={page === id ? "page" : undefined}
            onClick={() => choose(id)}
          >
            {label}
          </button>
        ))}
      </nav>
      {page === "home" && <OperatorHome choose={choose} />}
      {page === "catalog" && <CatalogConsole initialBranch={assignedBranch} />}
      {page === "finance" && <Finance initialBranch={assignedBranch} />}
      {page === "claims" && <EscrowClaims initialBranch={assignedBranch} />}
      {page === "disputes" && <EscrowDisputes initialBranch={assignedBranch} />}
      {page === "experience" && <Experience initialBranch={assignedBranch} />}
      {page === "payout" && <Payout initialBranch={assignedBranch} />}
      {page === "rental" && (
        <Fleet
          key="rental"
          mode="rental"
          initialBranch={assignedBranch}
          initialRentalScope={rentalScope}
          onReconcile={(scope) => {
            setRentalScope(scope);
            setAssignedBranch(scope.branch);
            setPage("v1");
          }}
        />
      )}
      {page === "v2" && (
        <DeploymentConsole key="v2" initialBranch={assignedBranch} mode="v2" />
      )}
      {page === "v1" && (
        <>
          <DeploymentConsole key="v1" initialBranch={assignedBranch} mode="v1" />
          {rentalScope && (
            <button type="button" onClick={() => setPage("rental")}>
              같은 대여 기기 반납 준비
            </button>
          )}
        </>
      )}
      {page === "fota" && (
        <Fleet key="fota" mode="fota" initialBranch={assignedBranch} />
      )}
    </main>
  );
}

function CatalogConsole({ initialBranch }: { initialBranch: string }) {
  const [branch, setBranch] = useState(initialBranch);
  const scopes = useAuthorizedBranches("operator");
  const [site, setSite] = useState<Site>();
  // The tax-free shop designation, read with the order state; the refund mark needs it.
  const [refundShop, setRefundShop] = useState<string>();
  const [draft, setDraft] = useState<Draft[]>([]);
  const [products, setProducts] = useState<Product[]>([]);
  // `at` is the place of an existing product; a new one has none.
  const [editing, setEditing] = useState<{ product: Product; at?: number; row?: number }>();
  // The rows of a product without groups, as they were when it was opened.
  const [rowsOf, setRowsOf] = useState<{ sku: string; before: Draft[]; photo?: PhotoInfo }>();
  // The photo of each product by its code, as it will be published.
  const [photos, setPhotos] = useState<Record<string, PhotoInfo>>({});
  const setPhoto = (sku: string, photo?: PhotoInfo) => setPhotos((old) => {
    const { [sku]: _, ...rest } = old;
    return photo ? { ...rest, [sku]: photo } : rest;
  });
  const photoSource = (photo: string) => `${endpoint(site!.branch)}/photo/${photo}`;
  async function uploadPhoto(file: File): Promise<PhotoInfo> {
    const image = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result).split(",")[1] ?? "");
      reader.onerror = () => reject(new Error("사진 파일을 읽지 못했습니다."));
      reader.readAsDataURL(file);
    });
    const reply = await fetch(`${endpoint(site!.branch)}/photo`, {
      method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ requestKey: crypto.randomUUID(), image, rightsConfirmed: true }),
    });
    if (reply.status === 422) throw new Error(`${file.name} · 사진을 읽을 수 없습니다. JPEG나 PNG인지 확인해 주세요.`);
    if (reply.status === 400) throw new Error("오늘 올린 게시 전 사진이 50장을 넘었습니다. 게시하거나 내일 다시 올려 주세요.");
    if (!reply.ok) throw new Error("사진을 올리지 못했습니다. 운영 권한을 확인하고 다시 시도해 주세요.");
    const made = await reply.json() as { photo: string; bytes: number };
    if (!/^[0-9a-f]{64}$/.test(made.photo)) throw new Error("사진을 올리지 못했습니다.");
    return { photo: made.photo, bytes: made.bytes, confirmedAt: new Date().toISOString() };
  }
  const photoField = (label: string, value: PhotoInfo | undefined, change: (photo?: PhotoInfo) => void) =>
    <PhotoField label={label} value={value} source={photoSource} upload={uploadPhoto} change={change} />;
  const [categoryDraft, setCategoryDraft] = useState<CategoryDraft>({ categories: [], categoryBindings: [] });
  const catalogSKUs = [...new Set([...draft, ...products].map((row) => row.sku).filter(Boolean))];
  const [deadline, setDeadline] = useState("");
  const [deadlineChoice, setDeadlineChoice] = useState("");
  const [hours, setHours] = useState<Hours>(defaultHours);
  const [limitedHours, setLimitedHours] = useState(false);
  const [sku, setSKU] = useState("");
  const [stockOption, setStockOption] = useState("");
  const [delta, setDelta] = useState("");
  const [confirm, setConfirm] = useState<Pending>();
  const [pending, setPending] = useState<Pending>();
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [stale, setStale] = useState(false);
  const operation = useRef(false);

  async function read(branch: string) {
    const record = (await exchange(endpoint(branch))) as Omit<Site, "branch">;
    if (
      !/^[0-9a-f]{64}$/.test(record.receiptScope) ||
      !Array.isArray(record.catalog?.rows)
    )
      throw new Error("UNKNOWN");
    return { ...record, branch };
  }

  function useSnapshot(exact: Site) {
    setSite(exact);
    setStale(false);
    setDraft(editableRows(exact));
    setProducts(exact.asset ? savedProducts(exact.catalog.options, exact.asset) : []);
    setEditing(undefined);
    setRowsOf(undefined);
    setPhotos(sitePhotos(exact));
    setCategoryDraft({ categories: exact.catalog.categories ?? [], categoryBindings: exact.catalog.categoryBindings ?? [] });
    setDeadline(
      exact.catalog.expiresAt
        ? localScheduleValue(exact.catalog.expiresAt, exact.catalog.timezone)
        : ""
    );
    setDeadlineChoice(exact.catalog.expiresAt ?? "");
    setLimitedHours(!!exact.catalog.hours);
    setHours(exact.catalog.hours ?? defaultHours);
  }

  function operate(run: () => Promise<void>) {
    if (operation.current) return;
    operation.current = true;
    setBusy(true);
    setNotice("");
    run()
      .catch((error: Error) =>
        setNotice(
          {
            DENIED: "이 지점의 활성 운영자 권한을 확인할 수 없습니다.",
            CONFLICT:
              "서버 상태가 바뀌었습니다. 제출 전 현재 버전을 확인하세요.",
            INVALID: "입력값과 서버의 게시 기간을 확인하세요. 옵션 묶음의 이름이 길거나 조합이 많으면 앱이 읽을 수 있는 크기를 넘을 수 있습니다.",
            STORAGE: "보호된 재시도 기록을 사용할 수 없습니다.",
          }[error.message] ??
            "응답을 확인하지 못했습니다. 같은 요청을 다시 확인하세요."
        )
      )
      .finally(() => {
        operation.current = false;
        setBusy(false);
      });
  }

  async function open() {
    const chosen = branch.trim();
    if (!uuid.test(chosen)) throw new Error("INVALID");
    setSite(undefined);
    setConfirm(undefined);
    setPending(undefined);
    const result = await read(chosen);
    setPending(scopedPending(result));
    useSnapshot(result);
  }

  const deadlineCandidates =
    site && deadline ? scheduleCandidates(deadline, site.catalog.timezone) : [];
  const deadlineISO =
    deadlineChoice ||
    (deadlineCandidates.length === 1 ? deadlineCandidates[0] : "");

  function propose(action: "publish" | "stock", event: FormEvent) {
    event.preventDefault();
    if (
      !site ||
      stale ||
      pending ||
      localStorage.getItem(pendingKey(site.receiptScope)) ||
      confirm
    )
      return;
    const body: Record<string, unknown> = {
      requestKey: crypto.randomUUID(),
      version: site.catalog.version,
    };
    if (action === "publish") {
      if (!site.asset || !(draft.length + products.length) || !deadlineISO) {
        setNotice("가격 행과 RFC 3339 만료 시간을 확인하세요.");
        return;
      }
      if (draft.some((row) => row.asset !== site.asset?.address)) {
        setNotice("가격 행의 결제 자산을 확인하세요.");
        return;
      }
      const problem = priceProblem(draft, site.asset);
      if (problem) {
        setNotice(problem);
        return;
      }
      const { asset } = site;
      if (menuLines(products, draft.length) > rowLimit) {
        setNotice(`매장 전체 ${menuLines(products, draft.length)}줄입니다. ${rowLimit}줄까지 게시할 수 있습니다.`);
        return;
      }
      const taken = new Set(draft.map(({ sku }) => sku));
      const wrong = products.find((product) => {
        const found = productProblems(product, asset);
        return found.incomplete || found.product || found.groups.some(Boolean) || taken.has(product.sku);
      });
      if (wrong) {
        setNotice(`${wrong.label}의 옵션 묶음과 상품 코드를 확인하세요.`);
        return;
      }
      const listed = Object.entries(photos).filter(([sku]) => catalogSKUs.includes(sku));
      if (listed.length > photoLimit) {
        setNotice(`메뉴 사진은 ${photoLimit}장까지 게시할 수 있습니다.`);
        return;
      }
      const grouped = publishedProducts(products, asset.address);
      const { categories, categoryBindings } = categoryDraft;
      const keys = new Set(categories.map(({ key }) => key));
      if (keys.size !== categories.length || categories.some(({ labels }) =>
        !labels.ko.trim() || labels.ko !== labels.ko.trim() ||
        !labels.en.trim() || labels.en !== labels.en.trim())) {
        setNotice("카테고리 코드는 중복 없이 입력하고 앞뒤 공백 없는 한국어·영어 이름을 입력하세요.");
        return;
      }
      Object.assign(body, {
        ...(categories.length || site.catalog.categories?.length ? {
          categories,
          categoryBindings: categoryBindings.filter(({ sku, categoryKey }) =>
            catalogSKUs.includes(sku) && keys.has(categoryKey)),
        } : {}),
        ...(limitedHours ? { hours } : {}),
        rows: [...draft.map(({ price, ...row }) => ({
          ...row,
          displayPrice: price,
        })), ...grouped.rows],
        ...(products.length ? { options: grouped.options } : {}),
        photos: listed.map(([sku, { photo }]) => ({ sku, photo })),
        expiresAt: new Date(deadlineISO).toISOString(),
        localExpiry: deadline,
      });
    } else {
      const adjustment = Number(delta);
      if (
        (!/^-?[1-9][0-9]{0,6}$/.test(delta) &&
          (delta !== "0" || !stockOption)) ||
        Math.abs(adjustment) > 1000000 ||
        !sku
      ) {
        setNotice("조정할 상품과 0이 아닌 정수 수량을 입력하세요.");
        return;
      }
      Object.assign(body, {
        sku,
        delta: adjustment,
        ...(stockOption ? { option: stockOption } : {}),
      });
    }
    setConfirm({
      branch: site.branch,
      scope: site.receiptScope,
      operation: action === "stock" && stockOption ? "option-stock" : action,
      body,
    });
  }

  async function send(command: Pending) {
    if (
      !site ||
      site.branch !== command.branch ||
      site.receiptScope !== command.scope
    )
      throw new Error("DENIED");
    const key = pendingKey(command.scope);
    const payload = JSON.stringify(command);
    const current = localStorage.getItem(key);
    if (current && current !== payload) throw new Error("STORAGE");
    localStorage.setItem(key, payload);
    const clearReceipt = () => {
      if (localStorage.getItem(key) === payload) localStorage.removeItem(key);
    };
    setPending(command);
    setConfirm(undefined);
    try {
      await exchange(
        `${endpoint(command.branch)}/${command.operation}`,
        command.body
      );
    } catch (error) {
      if (
        error instanceof Error &&
        ["CONFLICT", "INVALID"].includes(error.message)
      ) {
        clearReceipt();
        setPending(undefined);
        if (error.message === "CONFLICT") setStale(true);
      }
      throw error;
    }
    clearReceipt();
    setPending(undefined);
    useSnapshot(await read(command.branch));
    setNotice(
      command.operation !== "publish"
        ? "재고 반영을 확인했습니다."
        : "새 메뉴 버전의 게시를 확인했습니다."
    );
  }

  return (
    <>
      <header>
        <h1>지점 운영</h1>
      </header>
      <form
        className="branch"
        onSubmit={(event) => {
          event.preventDefault();
          operate(open);
        }}
      >
        <BranchPicker
          branches={scopes.filter((row) =>
            row.capabilities.includes("catalog")
          )}
          value={branch}
          disabled={busy}
          change={(value) => {
            setBranch(value);
            setSite(undefined);
            setConfirm(undefined);
            setPending(undefined);
          }}
        />
        <button disabled={busy}>운영 권한 및 메뉴 조회</button>
      </form>
      <p role="status">{notice}</p>
      {site && (
        <>
          <section>
            <h2>현재 메뉴</h2>
            <p>
              버전 {site.catalog.version} · 만료{" "}
              {site.catalog.expiresAt ? (
                <time
                  dateTime={site.catalog.expiresAt}
                  title={site.catalog.expiresAt}
                >
                  {localDeadline(site.catalog.expiresAt, site.catalog.timezone)}
                </time>
              ) : (
                "게시 대기"
              )}
            </p>
            {!!site.catalog.rows.length && (
              <ul className="live">
                {site.catalog.rows.map((row) => (
                  <li key={row.sku + "/" + row.option}>
                    <strong>{row.label}</strong> <span>{optionName(site, row)}</span>{" "}
                    <span>
                      {site.asset?.address === row.asset
                        ? tokenMoney(row.price, assetToken(site.asset))
                        : `${row.price} 기준 단위`}
                    </span>{" "}
                    <span>
                      공통 재고 {row.available}
                      {row.optionAvailable != null &&
                        ` · 옵션 재고 ${row.optionAvailable}`}
                    </span>
                    <span>
                      {menuDeclarationSummary(
                        site.catalog.breakdown?.find(
                          (item) =>
                            item.sku === row.sku && item.option === row.option
                        )?.metadata
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </section>
          {site.catalog.version > 0 && (
            <StoreState
              key={site.branch}
              branch={site.branch}
              zone={site.catalog.timezone}
              products={storeProducts(site.catalog.rows)}
              onState={(state) => setRefundShop(state.taxFreeShop?.designation)}
              money={site.asset ? (units) => tokenMoney(units, assetToken(site.asset!)) : undefined}
            />
          )}
          {stale && (
            <section aria-label="편집본 충돌">
              <p>
                편집본을 보관했습니다. 최신 버전을 불러온 뒤 변경 내역을 다시
                검토하세요.
              </p>
              <button
                disabled={busy}
                onClick={() =>
                  operate(async () => {
                    const latest = await read(site.branch);
                    if (latest.receiptScope !== site.receiptScope)
                      throw new Error("DENIED");
                    setSite(latest);
                    setStale(false);
                    setNotice(
                      "현재 버전을 갱신했습니다. 남겨둔 편집본과 비교해 주세요."
                    );
                  })
                }
              >
                편집본을 유지하고 최신 버전 조회
              </button>
            </section>
          )}
          {pending && (
            <section aria-label="응답 미확정">
              <h2>응답 미확정</h2>
              <p>저장한 요청을 재개해 서버 결과를 확인하세요.</p>
              <button
                disabled={busy}
                onClick={() => operate(() => send(pending))}
              >
                같은 요청 재조회
              </button>
            </section>
          )}
          {confirm && (
            <section aria-label="변경 확인">
              <h2>
                {confirm.operation !== "publish"
                  ? "재고 조정 확인"
                  : "메뉴 게시 확인"}
              </h2>
              <p>현재 버전 {site.catalog.version}에 적용합니다.</p>
              {confirm.operation === "publish" && (
                <>
                  <p>
                    게시 만료{" "}
                    {localDeadline(
                      String(confirm.body.expiresAt),
                      site.catalog.timezone
                    )}
                  </p>
                  <p>
                    영업 시간{" "}
                    {site.catalog.hours
                      ? Object.values(site.catalog.hours).join(" ")
                      : "제한 없음"}
                    {" → "}
                    {limitedHours
                      ? `${hours.opens}–${hours.closes} ${hours.zone}`
                      : "제한 없음"}
                  </p>
                  <p>이미 발급된 견적은 발급가를 유지합니다.</p>
                  {site.asset && <TaxReview asset={site.asset}
                    before={[...editableRows(site), ...savedProducts(site.catalog.options, site.asset)]}
                    after={[...draft.filter((row, index) => draft.findIndex(({ sku }) => sku === row.sku) === index), ...products]} />}
                  <PhotoReview before={sitePhotos(site)} after={Object.fromEntries(Object.entries(photos).filter(([sku]) => catalogSKUs.includes(sku)))}
                    names={Object.fromEntries([...draft, ...products].map(({ sku, label }) => [sku, label]))} source={photoSource} />
                  {site.asset && (!!products.length || !!site.catalog.options?.length) && (
                    <OptionReview before={savedProducts(site.catalog.options, site.asset)} after={products} asset={site.asset} lines={menuLines(products, draft.length)} />
                  )}
                  {Array.isArray(confirm.body.categories) && <CategoryReview value={confirm.body as CategoryDraft} skus={catalogSKUs} />}
                  <ul>
                    {catalogChanges(editableRows(site), draft).map(
                      ({ id, previous, updated }) => (
                        <li key={id}>
                          {id} ·{" "}
                          {previous
                            ? priceDescription(
                                previous,
                                site.asset
                              )
                            : "새 옵션"}{" "}
                          →{" "}
                          {updated
                            ? priceDescription(
                                updated,
                                site.asset
                              )
                            : "제거"}
                        </li>
                      )
                    )}
                  </ul>
                  <ul aria-label="게시 알레르기와 메뉴 선언">
                    {catalogChanges(editableRows(site), draft).map(
                      ({ id, previous, updated }) => (
                        <li key={id}>
                          {id} · {menuDeclarationSummary(previous?.metadata)} →{" "}
                          {menuDeclarationSummary(updated?.metadata)}
                        </li>
                      )
                    )}
                  </ul>
                </>
              )}
              {confirm.operation !== "publish" && (
                <p>
                  {String(confirm.body.sku)}
                  {!!confirm.body.option && ` ${optionName(site, { sku: String(confirm.body.sku), option: String(confirm.body.option) })}`} 수량 변경{" "}
                  {String(confirm.body.delta)}
                </p>
              )}
              <button
                disabled={busy}
                onClick={() => operate(() => send(confirm))}
              >
                변경 적용
              </button>
              <button disabled={busy} onClick={() => setConfirm(undefined)}>
                취소
              </button>
            </section>
          )}
          {site.asset && editing && (
            <ProductEditor
              key={editing.product.sku + editing.at}
              product={editing.product}
              asset={site.asset}
              refundShop={refundShop}
              fresh={editing.at === undefined}
              taken={[...products.filter((_, i) => i !== editing.at), ...draft.filter((_, i) => i !== editing.row)].map(({ sku }) => sku)}
              otherLines={menuLines(products.filter((_, i) => i !== editing.at), draft.length - (editing.row === undefined ? 0 : 1))}
              cancel={() => setEditing(undefined)}
              photo={photos[editing.product.sku]}
              photoField={photoField}
              remove={() => {
                setProducts((list) => list.filter((_, i) => i !== editing.at));
                setEditing(undefined);
              }}
              save={(product, photo) => {
                const { groups, ...plain } = product;
                setPhotos((old) => {
                  const { [editing.product.sku]: _, [product.sku]: __, ...rest } = old;
                  return photo ? { ...rest, [product.sku]: photo } : rest;
                });
                setProducts((list) => {
                  const rest = list.filter((_, i) => i !== editing.at);
                  if (!groups.length) return rest;
                  return editing.at === undefined ? [...list, product] : list.map((item, i) => (i === editing.at ? product : item));
                });
                setDraft((rows) => [
                  ...rows.filter((_, i) => i !== editing.row),
                  ...(groups.length ? [] : [{ ...plain, option: "standard", asset: site.asset!.address, required: false }]),
                ]);
                setEditing(undefined);
              }}
            />
          )}
          {site.asset && !editing && !rowsOf && (
            <fieldset className="bare" disabled={busy || stale || !!confirm || !!pending}>
              <ProductList
                products={products}
                plain={[...new Set(draft.map(({ sku }) => sku).filter(Boolean))].map((sku) => {
                  const rows = draft.filter((row) => row.sku === sku);
                  return { sku, label: rows[0].label, price: shownTotal(rows[0], site.asset!), lines: rows.length };
                })}
                asset={site.asset}
                lines={menuLines(products, draft.length)}
                edit={(at) => setEditing({ product: products[at], at })}
                add={() => setEditing({ product: emptyProduct() })}
                show={(sku) => setRowsOf({ sku, before: draft, photo: photos[sku] })}
              />
            </fieldset>
          )}
          {rowsOf && (
            <form aria-label="옵션 목록 고치기" onSubmit={(event) => { event.preventDefault(); setRowsOf(undefined); }}>
              <section>
                <h2>{draft.find(({ sku }) => sku === rowsOf.sku)?.label || rowsOf.sku}</h2>
                {photoField(draft.find(({ sku }) => sku === rowsOf.sku)?.label || rowsOf.sku, photos[rowsOf.sku], (photo) => setPhoto(rowsOf.sku, photo))}
                {site.asset && (() => {
                  const first = draft.find(({ sku }) => sku === rowsOf.sku);
                  // Every row of the product takes the same tax.
                  return first && <TaxField row={first} metadata={first.metadata} asset={site.asset} refundShop={refundShop}
                    change={(metadata, tax) => setDraft((rows) => rows.map((item) => item.sku !== rowsOf.sku ? item : {
                      ...item, tax, metadata: { ...item.metadata, tax: metadata.tax, takeAway: metadata.takeAway, taxRefund: metadata.taxRefund },
                    }))} />;
                })()}
                <fieldset className="bare" disabled={busy || stale || !!confirm || !!pending}>
                  {draft.map((row, index) => row.sku !== rowsOf.sku ? null : (
                  <fieldset key={index} data-sku={row.sku}>
                    <legend>옵션 {index + 1}</legend>
                    <div className="grid">
                      <label>
                        {names.asset}
                        <select
                          required
                          value={row.asset}
                          onChange={(event) =>
                            setDraft((old) =>
                              old.map((item, i) =>
                                i === index
                                  ? { ...item, asset: event.target.value }
                                  : item
                              )
                            )
                          }
                        >
                          <option value="">등록 자산 선택</option>
                          {site.asset && (
                            <option value={site.asset.address}>
                              {site.asset.symbol} · StableNet{" "}
                              {site.asset.chainId}
                            </option>
                          )}
                        </select>
                      </label>
                      {(
                        [
                          "sku",
                          "option",
                          "label",
                          "price",
                          "tax",
                          "discount",
                        ] as const
                      ).filter((key) => key !== "tax" || !row.metadata?.tax).map((key) => (
                        <label key={key}>
                          {names[key]}
                          <input
                            required={key !== "tax" && key !== "discount"}
                            inputMode={
                              ["price", "tax", "discount"].includes(key)
                                ? "decimal"
                                : undefined
                            }
                            value={row[key]}
                            onChange={(event) =>
                              setDraft((old) =>
                                old.map((item, i) =>
                                  i === index
                                    ? { ...item, [key]: event.target.value }
                                    : item
                                )
                              )
                            }
                          />
                        </label>
                      ))}
                      <label>
                        <input
                          type="checkbox"
                          checked={row.required}
                          onChange={(event) =>
                            setDraft((old) =>
                              old.map((item, i) =>
                                (i === index || (row.sku && item.sku === row.sku))
                                  ? { ...item, required: event.target.checked,
                                      metadata: {...item.metadata, optionGroupLabels: event.target.checked ? item.metadata?.optionGroupLabels : undefined} }
                                  : item
                              )
                            )
                          }
                        />
                        상품 구매 시 이 옵션군에서 선택 필수
                      </label>
                    </div>
                    <MenuDeclarations
                      metadata={row.metadata}
                      required={row.required}
                      change={(metadata) =>
                        setDraft((rows) =>
                          rows.map((item, i) =>
                            i === index ? { ...item, metadata } :
                              row.required && item.sku === row.sku && item.required
                                ? {...item, metadata: {...item.metadata, optionGroupLabels: metadata.optionGroupLabels}}
                                : item
                          )
                        )
                      }
                    />
                    {!row.required && draft.filter(({ sku }) => sku === row.sku).length === 1 && (
                      <button
                        type="button"
                        onClick={() => {
                          const { optionLabels: _, optionGroupLabels: __, ...metadata } = row.metadata ?? {};
                          setRowsOf(undefined);
                          setEditing({ row: index, product: { sku: row.sku, label: row.label, price: row.price, tax: row.tax, discount: row.discount,
                            metadata: row.metadata && metadata, groups: [emptyGroup()] } });
                        }}
                      >
                        옵션 묶음 만들기
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() =>
                        setDraft((rows) => rows.filter((_, i) => i !== index))
                      }
                    >
                      이 옵션 제거
                    </button>
                  </fieldset>
                  ))}
                  <button
                    type="button"
                    disabled={menuLines(products, draft.length) >= rowLimit}
                    onClick={() => setDraft((rows) => {
                      // A new option of a product keeps the product's tax, so one product never mixes two ways of taxing.
                      const same = rows.find((row) => row.sku === rowsOf.sku)?.metadata;
                      const tax = same?.tax ? { tax: same.tax, ...(same.takeAway ? { takeAway: true } : {}), ...(same.taxRefund ? { taxRefund: true } : {}) } : undefined;
                      return [...rows, { ...fresh(), sku: rowsOf.sku, asset: site.asset?.address ?? "", ...(tax ? { metadata: tax } : {}) }];
                    })}
                  >
                    옵션 추가
                  </button>
                </fieldset>
              </section>
              <section className="spread" aria-label="저장">
                <p>고친 것은 게시할 때 한꺼번에 적용됩니다.</p>
                <div>
                  <button type="button" className="quiet" onClick={() => { setDraft(rowsOf.before); setPhoto(rowsOf.sku, rowsOf.photo); setRowsOf(undefined); }}>되돌리기</button>
                  <button>이 상품 저장</button>
                </div>
              </section>
            </form>
          )}
          <section hidden={!!editing || !!rowsOf}>
            <h2>게시 설정</h2>
            {!site.asset && (
              <p>이 지점의 등록 결제 자산을 확인할 수 없습니다.</p>
            )}
            <form onSubmit={(event) => propose("publish", event)}>
              <fieldset
                disabled={
                  busy || stale || !!confirm || !!pending || !site.asset
                }
              >
                <label>
                  게시 만료 ({site.catalog.timezone})
                  <input
                    required
                    type="datetime-local"
                    value={deadline}
                    onChange={(event) => {
                      setDeadline(event.target.value);
                      setDeadlineChoice("");
                    }}
                  />
                </label>
                {deadline && !deadlineCandidates.length && (
                  <p role="status">
                    이 시간대에는 해당 시각이 없습니다. 다른 시각을 선택하세요.
                  </p>
                )}
                {deadlineCandidates.length > 1 && (
                  <label>
                    겹치는 시각의 UTC 차이 선택
                    <select
                      value={
                        deadlineChoice
                          ? new Date(deadlineChoice).toISOString().slice(0, 16)
                          : ""
                      }
                      onChange={(event) =>
                        setDeadlineChoice(
                          event.target.value
                            ? event.target.value + ":00.000Z"
                            : ""
                        )
                      }
                    >
                      <option value="">검토할 시각 선택</option>
                      {deadlineCandidates.map((candidate) => (
                        <option key={candidate} value={candidate.slice(0, 16)}>
                          {new Intl.DateTimeFormat("ko-KR", {
                            timeZone: site.catalog.timezone,
                            hour: "2-digit",
                            minute: "2-digit",
                            timeZoneName: "shortOffset",
                          }).format(new Date(candidate))}
                        </option>
                      ))}
                    </select>
                  </label>
                )}
                <label>
                  <input
                    type="checkbox"
                    checked={limitedHours}
                    onChange={(event) => setLimitedHours(event.target.checked)}
                  />
                  매일 영업 시간 지정
                </label>
                {limitedHours && (
                  <fieldset>
                    <legend>영업 시간</legend>
                    {(["opens", "closes", "zone"] as const).map((field) => (
                      <label key={field}>
                        {
                          {
                            opens: "영업 시작",
                            closes: "영업 종료",
                            zone: "시간대 (IANA)",
                          }[field]
                        }
                        <input
                          required
                          type={field === "zone" ? "text" : "time"}
                          value={hours[field]}
                          onChange={(event) =>
                            setHours({ ...hours, [field]: event.target.value })
                          }
                        />
                      </label>
                    ))}
                  </fieldset>
                )}
                <CatalogCategories value={categoryDraft} skus={catalogSKUs} change={setCategoryDraft} />
                <button disabled={!(draft.length + products.length)}>게시 내용 검토</button>
              </fieldset>
            </form>
          </section>
          <section>
            <h2>재고 조정</h2>
            <form onSubmit={(event) => propose("stock", event)}>
              <fieldset disabled={busy || !!confirm || !!pending}>
                <label>
                  상품{" "}
                  <select
                    value={sku}
                    onChange={(event) => {
                      setSKU(event.target.value);
                      setStockOption("");
                    }}
                    required
                  >
                    <option value="">상품 선택</option>
                    {[
                      ...new Map(
                        site.catalog.rows.map((row) => [row.sku, row.label])
                      ),
                    ].map(([value, label]) => (
                      <option key={value} value={value}>
                        {label} · {value}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  옵션별 한도
                  <select
                    value={stockOption}
                    onChange={(event) => setStockOption(event.target.value)}
                  >
                    <option value="">상품 공통 재고</option>
                    {site.catalog.rows
                      .filter((row) => row.sku === sku)
                      .map((row) => (
                        <option key={row.option} value={row.option}>
                          {optionName(site, row)}
                        </option>
                      ))}
                  </select>
                </label>
                <label>
                  증감 수량{" "}
                  <input
                    required
                    inputMode="numeric"
                    value={delta}
                    onChange={(event) => setDelta(event.target.value)}
                  />
                </label>
                <button disabled={!site.catalog.rows.length}>
                  재고 조정 검토
                </button>
              </fieldset>
            </form>
          </section>
        </>
      )}
    </>
  );
}
