import { DEFAULT_ENDPOINT, normalizeEndpoint, MCPError } from "./mcp-client.js";

const form = document.querySelector("#settings-form");
const endpointInput = document.querySelector("#endpoint");
const tokenInput = document.querySelector("#token");
const submitButton = document.querySelector("#submit");
const status = document.querySelector("#status");
const statusText = document.querySelector("#status-text");
const themeToggle = document.querySelector("#theme-toggle");
let currentTheme = "dark";

const stored = await chrome.storage.local.get(["settings", "webeggTheme"]);
endpointInput.value = stored.settings?.endpoint || DEFAULT_ENDPOINT;
tokenInput.value = stored.settings?.token || "";
applyTheme(stored.webeggTheme);

themeToggle.addEventListener("click", async () => {
  const next = currentTheme === "dark" ? "light" : "dark";
  applyTheme(next);
  await chrome.storage.local.set({ webeggTheme: next });
});
chrome.storage.onChanged.addListener((changes, area) => {
  if (area === "local" && changes.webeggTheme) applyTheme(changes.webeggTheme.newValue);
});

function applyTheme(theme) {
  currentTheme = theme === "light" ? "light" : "dark";
  document.body.dataset.theme = currentTheme;
  const label = currentTheme === "dark" ? "切换到日间模式" : "切换到夜间模式";
  themeToggle.setAttribute("aria-label", label);
  themeToggle.setAttribute("title", label);
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  setBusy(true);

  try {
    const endpoint = normalizeEndpoint(endpointInput.value);
    const token = tokenInput.value.trim();
    endpointInput.value = endpoint;
    await chrome.storage.local.set({ settings: { endpoint, token } });
    await chrome.runtime.sendMessage({ type: "webegg:poke-tabs" });
    setStatus("checking", "正在连接本机 WebEgg…");

    const result = await chrome.runtime.sendMessage({ type: "webegg:check-connection" });
    if (result?.ok) {
      setStatus("success", `已连接 ${result.name} ${result.version}`);
    } else {
      setStatus("error", result?.error?.message || result?.message || "连接检测失败，请重试");
    }
  } catch (error) {
    const message = error instanceof MCPError ? error.message : "无法保存设置，请重试";
    setStatus("error", message);
  } finally {
    setBusy(false);
  }
});

function setBusy(busy) {
  submitButton.disabled = busy;
  submitButton.classList.toggle("is-busy", busy);
  endpointInput.disabled = busy;
  tokenInput.disabled = busy;
}

function setStatus(kind, message) {
  status.className = `status ${kind}`;
  statusText.textContent = message;
}
