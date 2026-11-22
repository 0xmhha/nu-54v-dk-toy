import { useEffect, useState } from "react";

export type BranchScope = {
  id: string;
  label: string;
  timezone: string;
  capabilities: string[];
};

export function useAuthorizedBranches(audience: "operator" | "finance") {
  const [branches, setBranches] = useState<BranchScope[]>([]);
  useEffect(() => {
    const stop = new AbortController();
    setBranches([]);
    fetch(
      audience === "operator" ? "/v2/operations/scopes" : "/v2/finance/scopes",
      { credentials: "same-origin", signal: stop.signal }
    )
      .then((response) => (response.ok ? response.json() : []))
      .then(
        (rows: BranchScope[]) =>
          !stop.signal.aborted && setBranches(Array.isArray(rows) ? rows : [])
      )
      .catch(() => {
        if (!stop.signal.aborted) setBranches([]);
      });
    return () => stop.abort();
  }, [audience]);
  return branches;
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
