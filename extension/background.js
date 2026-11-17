import "./prompt.js";
import { callTool, checkConnection, DEFAULT_ENDPOINT, getClientConfig, listTools, MCPError, nextBridgeCommand, sendBridgeEvent } from "./mcp-client.js";

async function getSettings() {
  const stored = await chrome.storage.local.get("settings");
  return {
    endpoint: stored.settings?.endpoint || DEFAULT_ENDPOINT,
    token: stored.settings?.token || "",
  };
}

async function runConnectionCheck() {
  const settings = await getSettings();
  try {
    const connection = await checkConnection(settings);
    try {
      const clientConfig = await getClientConfig(settings);
      return { ...connection, ...clientConfig };
    } catch {
      return { ...connection, toolCallLimit: 0 };
    }
  } catch (error) {
    return publicError(error);
  }
}

async function runListTools() {
  const settings = await getSettings();
  try {
    const tools = await listTools(settings);
    return { ok: true, tools };
  } catch (error) {
    return publicError(error);
  }
}

async function runToolCall(message) {
  const settings = await getSettings();
  try {
    return await callTool({
      ...settings,
      name: message.name,
      arguments: message.arguments && typeof message.arguments === "object" ? message.arguments : {},
    });
  } catch (error) {
    return publicError(error);
  }
}

async function runBuildPrompt() {
  const settings = await getSettings();
  try {
    const tools = await listTools(settings);
    return { ok: true, prompt: globalThis.WebEggBuildPrompt(tools), count: tools.length };
  } catch (error) {
    return publicError(error);
  }
}

async function runBridgeNext(message) {
  const settings = await getSettings();
  try {
    const command = await nextBridgeCommand({ ...settings, provider: message.provider || "gemini", activeCommandId: message.activeCommandId || "" });
    let toolCallLimit = 0;
    try { toolCallLimit = (await getClientConfig(settings)).toolCallLimit; } catch { /* Older servers default to unlimited. */ }
    return { ok: true, command, toolCallLimit };
  } catch (error) {
    return publicError(error);
  }
}

async function runBridgeEvent(message) {
  const settings = await getSettings();
  try {
    await sendBridgeEvent({ ...settings, event: message.event });
    return { ok: true };
  } catch (error) {
    return publicError(error);
  }
}

async function pokeGeminiTabs() {
  const tabs = await chrome.tabs.query({ url: "https://gemini.google.com/*" });
  let awakened = 0;
  for (const tab of tabs) {
    if (!tab.id) continue;
    try {
      await chrome.tabs.sendMessage(tab.id, { type: "webegg:poke" });
      awakened++;
    } catch {
      try {
        await chrome.scripting.executeScript({ target: { tabId: tab.id }, files: ["gemini.js", "content.js"] });
        awakened++;
      } catch {
        // Restricted or still-loading tabs will be retried by the next alarm.
      }
    }
  }
  return { ok: true, tabs: tabs.length, awakened };
}

const KEEPALIVE_ALARM = "webegg-keepalive";
chrome.alarms.create(KEEPALIVE_ALARM, { periodInMinutes: 0.5 });
chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === KEEPALIVE_ALARM) void pokeGeminiTabs();
});
chrome.runtime.onStartup.addListener(() => { void pokeGeminiTabs(); });
chrome.runtime.onInstalled.addListener(() => { void pokeGeminiTabs(); });
void pokeGeminiTabs();

function publicError(error) {
  if (error instanceof MCPError) {
    return { ok: false, error: { code: error.code, message: error.message } };
  }
  return { ok: false, error: { code: "UNKNOWN", message: "WebEgg 请求失败，请重试" } };
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (sender.id !== chrome.runtime.id) return false;
  let task;
  switch (message?.type) {
    case "webegg:check-connection":
      task = runConnectionCheck();
      break;
    case "webegg:list-tools":
      task = runListTools();
      break;
    case "webegg:build-prompt":
      task = runBuildPrompt();
      break;
    case "webegg:bridge-next":
      task = runBridgeNext(message);
      break;
    case "webegg:bridge-event":
      task = runBridgeEvent(message);
      break;
    case "webegg:execute-tool":
      task = runToolCall(message);
      break;
    case "webegg:poke-tabs":
      task = pokeGeminiTabs();
      break;
    default:
      return false;
  }
  task.then(sendResponse);
  return true;
});
