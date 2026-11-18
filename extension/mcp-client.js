export const DEFAULT_ENDPOINT = "http://127.0.0.1:8787/mcp";
export const PROTOCOL_VERSION = "2026-07-28";

const CLIENT_INFO = { name: "webegg-extension", version: "0.9.8" };
const META = {
  "io.modelcontextprotocol/protocolVersion": PROTOCOL_VERSION,
  "io.modelcontextprotocol/clientInfo": CLIENT_INFO,
  "io.modelcontextprotocol/clientCapabilities": {},
};

export class MCPError extends Error {
  constructor(code, message, status = 0) {
    super(message);
    this.name = "MCPError";
    this.code = code;
    this.status = status;
  }
}

export function normalizeEndpoint(value) {
  let endpoint;
  try {
    endpoint = new URL(String(value || "").trim());
  } catch {
    throw new MCPError("INVALID_ENDPOINT", "请输入有效的 MCP 地址");
  }
  const localHosts = new Set(["127.0.0.1", "localhost", "[::1]"]);
  if (endpoint.protocol !== "http:" || !localHosts.has(endpoint.hostname)) {
    throw new MCPError("INVALID_ENDPOINT", "只允许连接本机 loopback HTTP 地址");
  }
  if (endpoint.username || endpoint.password || endpoint.search || endpoint.hash) {
    throw new MCPError("INVALID_ENDPOINT", "MCP 地址不能包含凭据、查询参数或锚点");
  }
  if (endpoint.pathname === "/") endpoint.pathname = "/mcp";
  if (endpoint.pathname !== "/mcp") {
    throw new MCPError("INVALID_ENDPOINT", "MCP 地址路径必须是 /mcp");
  }
  return endpoint.toString();
}

export function createDiscoveryRequest(endpoint, token, id = crypto.randomUUID()) {
  return createRPCRequest(endpoint, token, "server/discover", { _meta: META }, id);
}

export function createRPCRequest(endpointValue, token, method, params, id = crypto.randomUUID()) {
  const endpoint = normalizeEndpoint(endpointValue);
  if (!token || !token.startsWith("we_")) {
    throw new MCPError("INVALID_TOKEN", "请输入 WebEgg 生成的连接令牌");
  }
  const headers = {
    Authorization: `Bearer ${token}`,
    "Content-Type": "application/json",
    Accept: "application/json, text/event-stream",
    "MCP-Protocol-Version": PROTOCOL_VERSION,
    "Mcp-Method": method,
    "X-WebEgg-Client": "gemini-extension",
  };
  if (params?.name) headers["Mcp-Name"] = params.name;
  return {
    endpoint,
    init: {
      method: "POST",
      headers,
      body: JSON.stringify({
        jsonrpc: "2.0",
        id,
        method,
        params: { ...params, _meta: params?._meta || META },
      }),
    },
  };
}

export function parseResponse(text, contentType = "") {
  if (contentType.includes("text/event-stream")) {
    const events = text.split(/\r?\n\r?\n/);
    for (const event of events) {
      const data = event.split(/\r?\n/).filter((line) => line.startsWith("data:"))
        .map((line) => line.slice(5).trimStart()).join("\n");
      if (data) return parseJSON(data);
    }
    throw new MCPError("INVALID_RESPONSE", "MCP SSE 响应中没有数据");
  }
  return parseJSON(text);
}

export async function checkConnection({ endpoint, token, fetchImpl = fetch, timeoutMs = 5000 }) {
  const payload = await sendRPC({ endpoint, token, method: "server/discover", params: {}, fetchImpl, timeoutMs });
  const serverInfo = payload.result._meta?.["io.modelcontextprotocol/serverInfo"] || payload.result.serverInfo || {};
  return {
    ok: true,
    name: serverInfo.title || serverInfo.name || "MCP Server",
    version: serverInfo.version || "未知版本",
    protocolVersions: payload.result.supportedVersions || [],
  };
}

export async function listTools({ endpoint, token, fetchImpl = fetch, timeoutMs = 8000 }) {
  const payload = await sendRPC({ endpoint, token, method: "tools/list", params: {}, fetchImpl, timeoutMs });
  if (!Array.isArray(payload.result.tools)) {
    throw new MCPError("INVALID_RESPONSE", "MCP 工具列表格式不正确");
  }
  return payload.result.tools;
}

export async function getClientConfig({ endpoint, token, fetchImpl = fetch, timeoutMs = 5000 }) {
  createDiscoveryRequest(endpoint, token, 0);
  const url = new URL(normalizeEndpoint(endpoint));
  url.pathname = "/api/client-config";
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetchImpl(url, {
      headers: { Authorization: `Bearer ${token}`, "X-WebEgg-Client": "gemini-extension" },
      signal: controller.signal,
    });
    if (!response.ok) throw httpError(response.status);
    const payload = JSON.parse(await response.text());
    return { toolCallLimit: Number.isInteger(payload.toolCallLimit) && payload.toolCallLimit >= 0 ? payload.toolCallLimit : 0 };
  } finally {
    clearTimeout(timeout);
  }
}

export async function nextBridgeCommand({ endpoint, token, provider, activeCommandId = "", fetchImpl = fetch, timeoutMs = 10000 }) {
  createDiscoveryRequest(endpoint, token, 0);
  const url = new URL(normalizeEndpoint(endpoint));
  url.pathname = "/api/bridge/commands";
  url.search = `?provider=${encodeURIComponent(provider)}`;
  if (activeCommandId) url.search += `&activeCommandId=${encodeURIComponent(activeCommandId)}`;
  const response = await fetchImpl(url, {
    headers: { Authorization: `Bearer ${token}`, "X-WebEgg-Client": "gemini-extension" },
    signal: AbortSignal.timeout(timeoutMs),
  });
  if (!response.ok) throw httpError(response.status);
  const payload = JSON.parse(await response.text());
  return payload.command || null;
}

export async function sendBridgeEvent({ endpoint, token, event, fetchImpl = fetch, timeoutMs = 60000 }) {
  createDiscoveryRequest(endpoint, token, 0);
  const url = new URL(normalizeEndpoint(endpoint));
  url.pathname = "/api/bridge/events";
  const response = await fetchImpl(url, {
    method: "POST",
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json", "X-WebEgg-Client": "gemini-extension" },
    body: JSON.stringify(event),
    signal: AbortSignal.timeout(timeoutMs),
  });
  if (!response.ok) throw httpError(response.status);
  return true;
}

export async function callTool({
  endpoint,
  token,
  name,
  arguments: toolArguments,
  fetchImpl = fetch,
  timeoutMs = 620000,
  approvalPollMs = 1000,
  approvalTimeoutMs = 300000,
}) {
  if (!name?.startsWith("webegg_")) {
    throw new MCPError("INVALID_TOOL", "不允许调用非 WebEgg 工具");
  }
  const call = async (args) => {
    const payload = await sendRPC({
      endpoint, token, method: "tools/call",
      params: { name, arguments: args }, fetchImpl, timeoutMs,
    });
    return normalizeToolResult(payload.result);
  };

  let result = await call(toolArguments || {});
  const approvalID = result.error?.code === "APPROVAL_REQUIRED" ? result.error.approvalId : "";
  if (approvalID) {
    const decision = await waitForApproval({
      endpoint, token, approvalID, fetchImpl, approvalPollMs, approvalTimeoutMs,
    });
    if (decision === "rejected") {
      return { ok: false, error: { code: "APPROVAL_REJECTED", message: "用户已在桌面端拒绝这次操作" } };
    }
    result = await call({ ...(toolArguments || {}), approvalId: approvalID });
  }
  return result;
}

async function sendRPC({ endpoint, token, method, params, fetchImpl, timeoutMs }) {
  const request = createRPCRequest(endpoint, token, method, params);
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  let response;
  let text;
  try {
    response = await fetchImpl(request.endpoint, { ...request.init, signal: controller.signal });
    text = await response.text();
  } catch (error) {
    if (error?.name === "AbortError") {
      throw new MCPError("TIMEOUT", "连接或工具执行超时，请检查 WebEgg 状态");
    }
    throw new MCPError("OFFLINE", "无法读取本地服务响应，请确认 WebEgg 正在运行");
  } finally {
    clearTimeout(timeout);
  }

  let payload;
  try {
    payload = parseResponse(text, response.headers.get("content-type") || "");
  } catch (error) {
    if (!response.ok) throw httpError(response.status);
    throw error;
  }
  if (!response.ok) throw httpError(response.status);
  if (payload.error) throw new MCPError("MCP_ERROR", payload.error.message || "MCP 服务返回错误");
  if (!payload.result || payload.jsonrpc !== "2.0") {
    throw new MCPError("INVALID_RESPONSE", "MCP 服务响应格式不正确");
  }
  return payload;
}

function normalizeToolResult(result) {
  const text = result.content?.find((item) => item.type === "text")?.text;
  if (text) {
    try {
      const parsed = JSON.parse(text);
      return {
        ok: !result.isError && parsed.ok !== false,
        result: parsed.result,
        error: parsed.error,
        raw: parsed,
      };
    } catch {
      return { ok: !result.isError, result: text, raw: result };
    }
  }
  return { ok: !result.isError, result: result.structuredContent || result, raw: result };
}

async function waitForApproval({ endpoint, token, approvalID, fetchImpl, approvalPollMs, approvalTimeoutMs }) {
  const url = new URL(endpoint);
  url.pathname = "/api/approval-status";
  url.search = `?id=${encodeURIComponent(approvalID)}`;
  const deadline = Date.now() + approvalTimeoutMs;

  while (Date.now() < deadline) {
    try {
      const response = await fetchImpl(url, {
        headers: { Authorization: `Bearer ${token}`, "X-WebEgg-Client": "gemini-extension" },
      });
      if (response.ok) {
        const payload = JSON.parse(await response.text());
        if (payload.status === "approved" || payload.status === "rejected") return payload.status;
        if (payload.status === "not-found") {
          throw new MCPError("APPROVAL_EXPIRED", "审批已失效，请重新发起工具调用");
        }
      } else if (response.status === 401) {
        throw httpError(401);
      }
    } catch (error) {
      if (error instanceof MCPError) throw error;
      // A temporary desktop restart is reported through the normal approval timeout.
    }
    await new Promise((resolve) => setTimeout(resolve, approvalPollMs));
  }
  throw new MCPError("APPROVAL_TIMEOUT", "等待本地批准超时，未执行该操作");
}

function httpError(status) {
  if (status === 401) return new MCPError("INVALID_TOKEN", "令牌无效或已重置，请重新复制", status);
  if (status === 503) return new MCPError("TOKEN_NOT_CONFIGURED", "请先在 WebEgg 桌面端生成令牌", status);
  if (status === 403) return new MCPError("FORBIDDEN", "本地服务拒绝了请求来源", status);
  return new MCPError("HTTP_ERROR", `本地服务返回 HTTP ${status}`, status);
}

function parseJSON(text) {
  try {
    return JSON.parse(text);
  } catch {
    throw new MCPError("INVALID_RESPONSE", "无法解析 MCP 服务响应");
  }
}
