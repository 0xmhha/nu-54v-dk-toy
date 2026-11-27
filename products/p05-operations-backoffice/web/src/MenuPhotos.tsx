import { useId, useState } from "react";

/** A photo as the server made it: its name (SHA-256) and size. */
export type PhotoInfo = { photo: string; bytes?: number; confirmedAt?: string };

const accepted = ["image/jpeg", "image/png"];
const uploadLimit = 5 * 1024 * 1024;
const sideLimit = 4096;
export const photoLimit = 200;

const kb = (bytes?: number) => (bytes ? `${Math.round(bytes / 1024)}KB` : "");

/** Why a file cannot be sent, as far as the page can tell before the server reads it. */
export async function photoProblem(file: File): Promise<string | undefined> {
  if (!accepted.includes(file.type)) return `${file.name} · JPEG나 PNG만 올릴 수 있습니다. 휴대전화에서 JPEG로 내보낸 뒤 올려 주세요.`;
  if (file.size > uploadLimit) return `${file.name} · ${(file.size / 1048576).toFixed(1)}MB · 5MB를 넘습니다. 크기를 줄여서 올려 주세요.`;
  try {
    const picture = await createImageBitmap(file);
    const { width, height } = picture;
    picture.close();
    if (width > sideLimit || height > sideLimit) return `${file.name} · ${width.toLocaleString()}×${height.toLocaleString()} · 가로·세로는 4,096까지입니다.`;
  } catch {
    return `${file.name} · 사진을 읽을 수 없습니다.`;
  }
  return undefined;
}

export function PhotoField({ label, value, source, upload, change }: {
  label: string;
  value?: PhotoInfo;
  /** Where the page can show a photo of this branch. */
  source: (photo: string) => string;
  upload: (file: File) => Promise<PhotoInfo>;
  change: (photo?: PhotoInfo) => void;
}) {
  const [rights, setRights] = useState(false);
  const [busy, setBusy] = useState("");
  const [problem, setProblem] = useState("");
  const [replacing, setReplacing] = useState(false);
  const id = useId();
  async function send(file?: File) {
    setProblem("");
    if (!file) return;
    if (!rights) return setProblem("권리를 먼저 확인해 주세요. 확인하기 전에는 사진을 올리지 않습니다.");
    const found = await photoProblem(file);
    if (found) return setProblem(found);
    setBusy(`${file.name} · ${(file.size / 1048576).toFixed(1)}MB · 다시 만드는 중`);
    try {
      change(await upload(file));
      setReplacing(false);
      setRights(false);
    } catch (error) {
      setProblem(error instanceof Error && error.message ? error.message : "사진을 올리지 못했습니다. 다시 시도해 주세요.");
    } finally {
      setBusy("");
    }
  }
  return (
    <fieldset className="photo">
      <legend>메뉴 사진</legend>
      {value && !replacing ? (
        <div className="photo-shown">
          <img src={source(value.photo)} alt={`서버가 다시 만든 ${label} 메뉴 사진`} width={400} height={300} />
          <div>
            <dl className="totals">
              <dt>다시 만든 사진</dt><dd>800×600 JPEG{value.bytes ? ` · ${kb(value.bytes)}` : ""}</dd>
              <dt>지운 것</dt><dd>위치 정보, 촬영 기기 정보</dd>
              {value.confirmedAt && <><dt>권리 확인</dt><dd>{new Date(value.confirmedAt).toLocaleDateString("ko-KR")}</dd></>}
            </dl>
            <button type="button" className="quiet" onClick={() => setReplacing(true)}>다른 사진으로 바꾸기</button>
            <button type="button" className="quiet" onClick={() => change(undefined)}>사진 지우기</button>
          </div>
        </div>
      ) : (
        <div className="photo-add">
          {!value && <p><strong>{label} 사진이 없습니다.</strong> 사진이 없으면 키오스크와 휴대전화 앱에는 상품 이름만 보이고, 주문은 그대로 받습니다.</p>}
          <label>
            <input type="checkbox" checked={rights} onChange={(event) => setRights(event.target.checked)} />
            이 사진을 메뉴에 쓸 권리가 매장에 있습니다. 매장이 찍었거나 쓰도록 허락받은 사진입니다.
          </label>
          <label htmlFor={id} className="file">사진 고르기</label>
          <input id={id} type="file" accept="image/jpeg,image/png" disabled={!!busy}
            onChange={(event) => { const file = event.target.files?.[0]; event.target.value = ""; void send(file); }} />
          <p>JPEG·PNG, 5MB까지, 가로·세로 4,096까지. 서버가 800×600 JPEG로 다시 만들고 위치 정보와 숨은 자료를 지웁니다. 목록에는 작은 사진을 따로 만들어 보냅니다. 올린 원본은 남기지 않습니다.</p>
          {replacing && <button type="button" className="quiet" onClick={() => setReplacing(false)}>바꾸지 않기</button>}
        </div>
      )}
      {busy && <p role="status">{busy}</p>}
      {problem && <p role="alert" className="problem">{problem}</p>}
      {value && <p className="note">게시하면 키오스크와 휴대전화 앱에 보입니다. 게시하기 전에는 지금 사진이 그대로 보입니다.</p>}
    </fieldset>
  );
}

/** What publishing changes in the photos, product by product. */
export function PhotoReview({ before, after, names, source }: {
  before: Record<string, PhotoInfo>;
  after: Record<string, PhotoInfo>;
  names: Record<string, string>;
  source: (photo: string) => string;
}) {
  const skus = [...new Set([...Object.keys(before), ...Object.keys(after)])].filter((sku) => names[sku] !== undefined || before[sku]).sort();
  const word = (sku: string) => !before[sku] ? "새 사진" : !after[sku] ? "사진 지움" : before[sku].photo === after[sku].photo ? "그대로" : "다른 사진으로 바꿈";
  const changed = skus.filter((sku) => word(sku) !== "그대로");
  if (!changed.length) return null;
  return (
    <div>
      <h3>사진에서 바뀌는 것</h3>
      <ul aria-label="사진에서 바뀌는 것" className="live">
        {changed.map((sku) => (
          <li key={sku} className="spread">
            {after[sku] ? <img src={source(after[sku].photo)} alt={`${names[sku] ?? sku} 메뉴 사진`} width={96} height={72} /> : <span className="nophoto">사진 없음</span>}
            <strong>{names[sku] ?? sku}</strong>
            <span>{word(sku)}{!after[sku] && " · 이름만 보임"}</span>
          </li>
        ))}
      </ul>
      <p>메뉴 사진 {Object.keys(after).length} / {photoLimit}장. 게시한 뒤에 키오스크와 휴대전화가 새 사진을 받습니다. 받기 전이나 받다가 끊기면 이름만 보이고, 주문은 막지 않습니다.</p>
    </div>
  );
}
