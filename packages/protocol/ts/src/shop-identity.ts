const reference =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const entrance = "nu54://shop/";

export type ShopIdentity = {
  branch: string;
  branchLabel: string;
  terminal?: string;
};

export function makeEntranceQr(branch: string) {
  if (!reference.test(branch)) throw new Error("SHOP_ENTRANCE_QR");
  return entrance + branch;
}

export function readEntranceQr(raw: string) {
  const branch = raw.slice(entrance.length);
  if (
    !raw.startsWith(entrance) ||
    !reference.test(branch) ||
    raw.length !== entrance.length + 36
  )
    throw new Error("SHOP_ENTRANCE_QR");
  return branch;
}

export function readShopIdentity(
  raw: unknown,
  branch: string,
  terminal?: string
): ShopIdentity {
  const value = raw as ShopIdentity;
  if (
    !value ||
    !reference.test(branch) ||
    value.branch !== branch ||
    value.terminal !== terminal ||
    (terminal !== undefined && !reference.test(terminal)) ||
    typeof value.branchLabel !== "string" ||
    value.branchLabel !== value.branchLabel.trim() ||
    !value.branchLabel ||
    Array.from(value.branchLabel).length > 120 ||
    /[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/u.test(
      value.branchLabel
    ) ||
    Object.keys(value).sort().join(",") !==
      (terminal === undefined
        ? "branch,branchLabel"
        : "branch,branchLabel,terminal")
  )
    throw new Error("SHOP_IDENTITY");
  return value;
}
