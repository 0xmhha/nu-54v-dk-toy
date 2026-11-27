"use strict";

const hex = bytes => Array.from(new Uint8Array(bytes), value => value.toString(16).padStart(2, "0")).join("");
const sha256 = async bytes => hex(await crypto.subtle.digest("SHA-256", bytes));

function publicMaterial(text) {
  const match = text.trim().match(/^-----BEGIN PUBLIC KEY-----\s+([A-Za-z0-9+/=\s]+?)\s+-----END PUBLIC KEY-----$/);
  if (!match) throw new Error("key");
  return Uint8Array.from(atob(match[1].replace(/\s+/g, "")), char => char.charCodeAt(0));
}

async function selected(form, name, limit) {
  const file = form.elements[name].files[0];
  if (!file || file.size === 0 || file.size > limit) throw new Error(name);
  return file.arrayBuffer();
}

async function verifyArchive(form) {
  const item = form.elements.artifact.selectedOptions[0];
  const policy = form.dataset;
  const manifest = await selected(form, "manifest", 512 * 1024);
  const publicKey = publicMaterial(new TextDecoder("utf-8", {fatal: true}).decode(await selected(form, "key", 4096)));
  if (await sha256(publicKey) !== policy.signer) throw new Error("key");
  if (await sha256(manifest) !== policy.manifest) throw new Error("manifest");
  const sig = await selected(form, "signature", 64);
  if (sig.byteLength !== 64) throw new Error("signature");
  const key = await crypto.subtle.importKey("spki", publicKey, {name: "Ed25519"}, false, ["verify"]);
  if (!await crypto.subtle.verify("Ed25519", key, sig, manifest)) throw new Error("signature");
  const release = JSON.parse(new TextDecoder("utf-8", {fatal: true}).decode(manifest));
  if (release.format !== "nu54-release-evidence-v1" || release.channel !== policy.channel) throw new Error("manifest");
  const matches = release.artifacts.filter(entry => entry.id === item.value &&
    entry.version === item.dataset.version && entry.payload.sha256 === item.dataset.payload);
  if (matches.length !== 1) throw new Error("archive");
  if (await sha256(await selected(form, "archive", 64 * 1024 * 1024)) !== item.dataset.payload) throw new Error("archive");
  const path = "/api/D02?" + new URLSearchParams({cohort: policy.scope});
  const response = await fetch(path, {cache: "no-store", credentials: "same-origin"});
  if (!response.ok) throw new Error("released");
  const published = await response.json();
  if (published.manifest !== policy.manifest || published.releaseSigner !== policy.signer ||
      published.channel !== policy.channel) throw new Error("released");
}

for (const form of document.querySelectorAll("[data-local-verify]")) {
  const result = form.querySelector("output");
  const button = form.querySelector("button");
  const stale = () => {
    result.textContent = "선택한 자료를 다시 검증하세요.";
    result.dataset.state = "pending";
  };
  form.addEventListener("change", stale);
  form.addEventListener("submit", async event => {
    event.preventDefault();
    const inputs = [...form.elements].filter(element => element !== result);
    try {
      button.disabled = true;
      result.textContent = "로컬 파일을 검증하는 중입니다.";
      const pending = verifyArchive(form);
      for (const input of inputs) input.disabled = true;
      await pending;
      result.textContent = "출처 검증 완료 · 등록된 릴리스 서명과 로컬 아카이브 해시가 일치합니다.";
      result.dataset.state = "valid";
    } catch (error) {
      const details = {
        archive: "SDK 아카이브의 해시 또는 선택한 아티팩트가 다릅니다.",
        key: "공개 키 지문이 이 채널의 등록 키와 다릅니다.",
        signature: "등록된 공개 키로 manifest 서명을 확인할 수 없습니다.",
        manifest: "manifest 해시 또는 배포 채널이 등록된 릴리스와 다릅니다.",
        released: "현재 릴리스 승인이 변경되었습니다. 공개 릴리스를 다시 선택하세요.",
      };
      result.textContent = "설치 중단 · 이 산출물의 출처를 다시 확인하세요. " +
        (details[error.message] || "자료의 형식이나 로컬 검증 기능을 확인하세요.");
      result.dataset.state = "invalid";
    } finally {
      for (const input of inputs) input.disabled = false;
    }
  });
}
