import test from "node:test";
import assert from "node:assert/strict";

import {
  callTool,
  checkConnection,
  createDiscoveryRequest,
  DEFAULT_ENDPOINT,
  getClientConfig,
  listTools,
  MCPError,
  normalizeEndpoint,
  parseResponse,
  PROTOCOL_VERSION,
} from "../mcp-client.js";

test("normalizeEndpoint accepts loopback and supplies /mcp", () => {
  assert.equal(normalizeEndpoint("http://127.0.0.1:8787"), DEFAULT_ENDPOINT);
  assert.equal(normalizeEndpoint("http://localhost:8787/mcp"), "http://localhost:8787/mcp");
});

test("normalizeEndpoint rejects remote and unexpected URLs", () => {
  for (const value of [
    "https://example.com/mcp",
    "http://192.168.1.2:8787/mcp",
    "http://127.0.0.1:8787/other",
    "http://user:pass@127.0.0.1:8787/mcp",
    "not a url",
  ]) {
    assert.throws(() => normalizeEndpoint(value), (error) => error.code === "INVALID_ENDPOINT");
  }
});

test("discovery request carries current protocol metadata and headers", () => {
  const request = createDiscoveryRequest(DEFAULT_ENDPOINT, "we_test", 42);
  assert.equal(request.init.headers["MCP-Protocol-Version"], PROTOCOL_VERSION);
  assert.equal(request.init.headers["Mcp-Method"], "server/discover");
  assert.equal(request.init.headers["X-WebEgg-Client"], "gemini-extension");
  const body = JSON.parse(request.init.body);
  assert.equal(body.id, 42);
  assert.equal(body.method, "server/discover");
  assert.equal(
    body.params._meta["io.modelcontextprotocol/protocolVersion"],
    PROTOCOL_VERSION,
  );
});

test("parseResponse handles JSON and SSE", () => {
  const json = parseResponse('{"jsonrpc":"2.0","id":1,"result":{}}', "application/json");
  assert.equal(json.id, 1);
  const sse = parseResponse(
    'event: message\r\ndata: {"jsonrpc":"2.0","id":2,"result":{}}\r\n\r\n',
    "text/event-stream; charset=utf-8",
  );
  assert.equal(sse.id, 2);
});

test("checkConnection reads discovery server info", async () => {
  const result = await checkConnection({
    endpoint: DEFAULT_ENDPOINT,
    token: "we_test",
    fetchImpl: async () => new Response(JSON.stringify({
      jsonrpc: "2.0",
      id: 1,
      result: {
        supportedVersions: [PROTOCOL_VERSION],
        _meta: {
          "io.modelcontextprotocol/serverInfo": { name: "webegg", title: "WebEgg", version: "0.9.8" },
        },
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } }),
  });
  assert.deepEqual(result, {
    ok: true,
    name: "WebEgg",
    version: "0.9.8",
    protocolVersions: [PROTOCOL_VERSION],
  });
});

test("checkConnection maps authentication and offline errors", async () => {
  await assert.rejects(
    checkConnection({
      endpoint: DEFAULT_ENDPOINT,
      token: "we_wrong",
      fetchImpl: async () => new Response('{"error":"invalid_token"}', {
        status: 401,
        headers: { "Content-Type": "application/json" },
      }),
    }),
    (error) => error instanceof MCPError && error.code === "INVALID_TOKEN",
  );

  await assert.rejects(
    checkConnection({
      endpoint: DEFAULT_ENDPOINT,
      token: "we_test",
      fetchImpl: async () => { throw new TypeError("fetch failed"); },
    }),
    (error) => error.code === "OFFLINE",
  );
});

test("getClientConfig reads unlimited or configured tool call limit", async () => {
  const result = await getClientConfig({
    endpoint: DEFAULT_ENDPOINT,
    token: "we_test",
    fetchImpl: async (url) => {
      assert.equal(new URL(url).pathname, "/api/client-config");
      return new Response('{"toolCallLimit":0}', { status: 200, headers: { "Content-Type": "application/json" } });
    },
  });
  assert.deepEqual(result, { toolCallLimit: 0 });
});

test("listTools returns the actual MCP registry", async () => {
  const tools = await listTools({
    endpoint: DEFAULT_ENDPOINT,
    token: "we_test",
    fetchImpl: async () => rpcResponse({ tools: [{ name: "webegg_file_read", inputSchema: { type: "object" } }] }),
  });
  assert.equal(tools[0].name, "webegg_file_read");
});

test("callTool waits for approval and retries once with approvalId", async () => {
  let retried = false;
  const result = await callTool({
    endpoint: DEFAULT_ENDPOINT,
    token: "we_test",
    name: "webegg_file_create",
    arguments: { projectId: "proj_1", path: "test.txt", content: "ok" },
    approvalPollMs: 1,
    approvalTimeoutMs: 30,
    fetchImpl: async (url, init = {}) => {
      if (String(url).includes("/api/approval-status")) {
        return new Response('{"status":"approved"}', { status: 200, headers: { "Content-Type": "application/json" } });
      }
      const body = JSON.parse(init.body);
      if (!body.params.arguments.approvalId) {
        return rpcResponse({
          isError: true,
          content: [{ type: "text", text: JSON.stringify({ ok: false, error: { code: "APPROVAL_REQUIRED", approvalId: "approval_1" } }) }],
        });
      }
      retried = true;
      assert.equal(body.params.arguments.approvalId, "approval_1");
      return rpcResponse({ content: [{ type: "text", text: JSON.stringify({ ok: true, result: { created: true } }) }] });
    },
  });
  assert.equal(retried, true);
  assert.equal(result.ok, true);
  assert.deepEqual(result.result, { created: true });
});

test("checkConnection maps timeouts", async () => {
  await assert.rejects(
    checkConnection({
      endpoint: DEFAULT_ENDPOINT,
      token: "we_test",
      fetchImpl: async (_url, { signal }) => new Promise((_resolve, reject) => {
        signal.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")));
      }),
      timeoutMs: 5,
    }),
    (error) => error.code === "TIMEOUT",
  );
});

function rpcResponse(result) {
  return new Response(JSON.stringify({ jsonrpc: "2.0", id: 1, result }), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
