import { useEffect, useState } from "react";

export type BranchScope = {
  id: string;
  label: string;
  timezone: string;
  capabilities: string[];
};

export type BranchAccess = "loading" | "ready" | "failed";

export function useAuthorizedBranches(audience: "operator" | "finance") {
  return useBranchAccess(audience).branches;
}

/** The same scopes, with whether they were read, so a page can tell no access from no reply. */
export function useBranchAccess(audience: "operator" | "finance") {
  const [branches, setBranches] = useState<BranchScope[]>([]);
  const [access, setAccess] = useState<BranchAccess>("loading");
  useEffect(() => {
    const stop = new AbortController();
    setBranches([]);
    setAccess("loading");
    fetch(
      audience === "operator" ? "/v2/operations/scopes" : "/v2/finance/scopes",
      { credentials: "same-origin", signal: stop.signal }
    )
      .then((response) => {
        // 404 is the normal answer for an account without this kind of access.
        if (response.status !== 404 && !response.ok) throw new Error("SCOPES");
        return response.ok ? response.json() : [];
      })
      .then((rows: BranchScope[]) => {
        if (stop.signal.aborted) return;
        setBranches(Array.isArray(rows) ? rows : []);
        setAccess("ready");
      })
      .catch(() => {
        if (stop.signal.aborted) return;
        setBranches([]);
        setAccess("failed");
      });
    return () => stop.abort();
  }, [audience]);
  return { branches, access };
}

export function BranchPicker({
  branches,
  value,
  change,
  disabled,
}: {
  branches: BranchScope[];
  value: string;
  change: (value: string) => void;
  disabled: boolean;
}) {
  return (
    <>
      <label>
        담당 지점
        <select
          value={branches.some((row) => row.id === value) ? value : ""}
          disabled={disabled}
          onChange={(event) => change(event.target.value)}
        >
          <option value="">담당 지점 선택</option>
          {branches.map((row) => (
            <option key={row.id} value={row.id}>
              {row.label} · {row.timezone}
            </option>
          ))}
        </select>
      </label>
      <details>
        <summary>지점 참조로 진단</summary>
        <label>
          지점 참조
          <input
            disabled={disabled}
            value={value}
            maxLength={36}
            onChange={(event) => change(event.target.value)}
            autoComplete="off"
          />
        </label>
      </details>
    </>
  );
}
