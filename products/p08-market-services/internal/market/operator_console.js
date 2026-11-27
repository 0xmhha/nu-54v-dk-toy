const statusPanel = document.querySelector("#authorization");
const title = document.querySelector("#access-title");
const message = document.querySelector("#access-message");
const observation = document.querySelector("#observation");
const checkButton = document.querySelector("#recheck");

let request;
let timer;

function clearObservation() {
  clearTimeout(timer);
  request?.abort();
  observation.replaceChildren();
  observation.hidden = true;
  statusPanel.hidden = false;
  title.textContent = "Checking market operator access";
  message.textContent = "Price and keeper evidence requires an approved market operator grant.";
  checkButton.hidden = true;
}

function showFailure(denied) {
  clearObservation();
  title.textContent = denied ? "Cannot open market operations" : "Market operations unavailable";
  message.textContent = denied
    ? "Market operator access is required. Contact the deployment administrator before checking again."
    : "The console could not verify access or load the latest evidence. Check access again to reconnect.";
  checkButton.textContent = "Check operator access again";
  checkButton.hidden = false;
}

async function checkAccess() {
  clearObservation();
  if (document.hidden) return;
  const attempt = new AbortController();
  request = attempt;
  const { signal } = attempt;
  const get = (path, timeout, headers = {}) => fetch(path, {
    cache: "no-store",
    redirect: "error",
    headers,
    signal: AbortSignal.any([signal, AbortSignal.timeout(timeout)]),
  });

  async function verifyAccess() {
    const result = await get("/access", 2000);
    if (signal.aborted) return false;
    if (result.status === 204) return true;
    showFailure(result.status === 403 || result.status === 404);
    return false;
  }

  async function monitorAccess() {
    try {
      if (await verifyAccess()) timer = setTimeout(monitorAccess, 2000);
    } catch {
      if (!signal.aborted) showFailure(false);
    }
  }

  try {
    if (!(await verifyAccess())) return;
    const result = await get("/observation", 13000, { "X-P08-Console": "observation" });
    if (signal.aborted) return;
    if (result.status !== 200) {
      showFailure(result.status === 403 || result.status === 404);
      return;
    }
    const html = await result.text();
    if (!(await verifyAccess()) || signal.aborted) return;
    // This same-origin fragment is rendered by Go's HTML template for the verified grant.
    observation.innerHTML = html;
    observation.hidden = false;
    statusPanel.hidden = true;
    checkButton.textContent = "Refresh observation";
    checkButton.hidden = false;
    timer = setTimeout(monitorAccess, 2000);
  } catch {
    if (!signal.aborted) showFailure(false);
  }
}

checkButton.addEventListener("click", checkAccess);
document.addEventListener("visibilitychange", checkAccess);
window.addEventListener("pagehide", clearObservation);
window.addEventListener("pageshow", checkAccess);
