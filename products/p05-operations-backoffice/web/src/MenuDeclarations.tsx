import { commerceAllergens, type CommerceMenuMetadata } from "@nu54/protocol";

const allergenNames = [
  "우유",
  "달걀",
  "생선",
  "갑각류·패류",
  "견과류",
  "땅콩",
  "밀",
  "대두",
  "참깨",
  "메밀",
  "복숭아",
  "토마토",
  "아황산류",
  "호두",
  "잣",
  "돼지고기",
  "쇠고기",
  "닭고기",
  "오징어",
];

export function menuDeclarationSummary(metadata?: CommerceMenuMetadata) {
  const names = commerceAllergens.flatMap((code, index) =>
    metadata?.allergens?.includes(code) ? [allergenNames[index]] : []
  );
  const crossContactNames = commerceAllergens.flatMap((code, index) =>
    metadata?.crossContactAllergens?.includes(code) ? [allergenNames[index]] : []
  );
  const contact = {
    unknown: "미확인",
    possible: "가능성 있음",
    "declared-none": "점주가 없다고 표시",
  };
  return [
    metadata?.labels?.ko,
    metadata?.labels?.en,
    metadata?.optionLabels?.ko,
    metadata?.optionLabels?.en,
    metadata?.optionGroupLabels?.ko,
    metadata?.optionGroupLabels?.en,
    metadata?.allergens === undefined
      ? "알레르기 정보 미입력"
      : names.length
      ? names.join(", ")
      : "점주 신고 성분 없음",
    metadata?.crossContact ? contact[metadata.crossContact] : "교차접촉 미입력",
    metadata?.crossContactSource === "shared-equipment" ? "공용 제조 설비" :
      metadata?.crossContactSource === "shared-baking-equipment" ? "공용 제빵 설비" : undefined,
    crossContactNames.length ? "교차접촉 신고 성분 " + crossContactNames.join(", ") : undefined,
  ]
    .filter(Boolean)
    .join(" · ");
}

export function MenuDeclarations({
  metadata = {},
  change,
  required = false,
}: {
  metadata?: CommerceMenuMetadata;
  required?: boolean;
  change: (next: CommerceMenuMetadata) => void;
}) {
  const labelFields = ["labels", "optionLabels", "optionGroupLabels"] as const;
  const languages = ["ko", "en"] as const;
  return (
    <fieldset>
      <legend>언어별 메뉴와 점주 알레르기 정보</legend>
      {labelFields.filter(field => required || field !== "optionGroupLabels").map((field) =>
        languages.map((language) => (
          <label key={field + language}>
            {field === "labels" ? "상품" : field === "optionLabels" ? "옵션" : "필수 옵션 그룹"}{" "}
            {language === "ko" ? "한국어" : "영어"} 이름
            <input
              value={metadata[field]?.[language] ?? ""}
              maxLength={120}
              onChange={(event) => {
                const next = { ...metadata[field] };
                if (event.target.value) next[language] = event.target.value;
                else delete next[language];
                change({ ...metadata, [field]: Object.keys(next).length ? next : undefined });
              }}
            />
          </label>
        ))
      )}
      <label>
        <input
          type="checkbox"
          checked={metadata.allergens !== undefined}
          onChange={(event) =>
            change({
              ...metadata,
              allergens: event.target.checked ? [] : undefined,
            })
          }
        />
        점주 제공 성분을 게시
      </label>
      {metadata.allergens !== undefined && (
        <div className="grid">
          {commerceAllergens.map((code, index) => (
            <label key={code}>
              <input
                type="checkbox"
                checked={!!metadata.allergens?.includes(code)}
                onChange={(event) => {
                  const current = metadata.allergens ?? [];
                  change({
                    ...metadata,
                    allergens: event.target.checked
                      ? [...current, code]
                      : current.filter((item) => item !== code),
                  });
                }}
              />
              {allergenNames[index]}
            </label>
          ))}
        </div>
      )}
      <label>
        교차접촉 정보
        <select
          aria-label="교차접촉 정보"
          value={metadata.crossContact ?? ""}
          onChange={(event) => {
            const status = (event.target.value || undefined) as CommerceMenuMetadata["crossContact"];
            change({
              ...metadata,
              crossContact: status,
              crossContactSource: status === "possible" ? metadata.crossContactSource : undefined,
              crossContactAllergens: status === "possible" ? metadata.crossContactAllergens : undefined,
            });
          }}
        >
          <option value="">미입력</option>
          <option value="unknown">점주 확인 전</option>
          <option value="possible">가능성 있음</option>
          <option value="declared-none">점주가 없다고 신고</option>
        </select>
      </label>
      {metadata.crossContact === "possible" && (
        <fieldset>
          <legend>점주 신고 교차접촉 정보</legend>
          <label>
            교차접촉 설비
            <select aria-label="교차접촉 설비" value={metadata.crossContactSource ?? ""}
              onChange={event => change({ ...metadata, crossContactSource: (event.target.value || undefined) as CommerceMenuMetadata["crossContactSource"] })}>
              <option value="">미입력</option>
              <option value="shared-equipment">공용 제조 설비</option>
              <option value="shared-baking-equipment">공용 제빵 설비</option>
            </select>
          </label>
          <div className="grid">
            {commerceAllergens.map((code, index) => (
              <label key={code}>
                <input type="checkbox" checked={!!metadata.crossContactAllergens?.includes(code)}
                  onChange={event => {
                    const current = metadata.crossContactAllergens ?? [];
                    const next = event.target.checked ? [...current, code] : current.filter(item => item !== code);
                    change({ ...metadata, crossContactAllergens: next.length ? next : undefined });
                  }} />
                교차접촉 {allergenNames[index]}
              </label>
            ))}
          </div>
        </fieldset>
      )}
    </fieldset>
  );
}
