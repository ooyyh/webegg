import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { DEFAULT_ENDPOINT, nextBridgeCommand, sendBridgeEvent } from "../mcp-client.js";

test("nextBridgeCommand retrieves queued provider work", async () => {
  const command = await nextBridgeCommand({
    endpoint: DEFAULT_ENDPOINT,
    token: "we_test",
    provider: "gemini",
    activeCommandId: "cmd_active",
    fetchImpl: async (url) => {
      const parsed = new URL(url);
      assert.equal(parsed.pathname, "/api/bridge/commands");
      assert.equal(parsed.searchParams.get("provider"), "gemini");
      assert.equal(parsed.searchParams.get("activeCommandId"), "cmd_active");
      return new Response(JSON.stringify({ command: { id: "cmd_1", prompt: "读取项目", initialize: true } }), {
        status: 200, headers: { "Content-Type": "application/json" },
      });
    },
  });
  assert.equal(command.id, "cmd_1");
  assert.equal(command.initialize, true);
});

test("sendBridgeEvent posts local timeline events", async () => {
  let body;
  await sendBridgeEvent({
    endpoint: DEFAULT_ENDPOINT,
    token: "we_test",
    event: { type: "assistant.done", commandId: "cmd_1", text: "完成" },
    fetchImpl: async (url, init) => {
      assert.equal(new URL(url).pathname, "/api/bridge/events");
      body = JSON.parse(init.body);
      return new Response('{"accepted":true}', { status: 200, headers: { "Content-Type": "application/json" } });
    },
  });
  assert.equal(body.type, "assistant.done");
  assert.equal(body.text, "完成");
});

test("Gemini content script is transport-only and injects no WebEgg UI", () => {
  const manifest = JSON.parse(readFileSync(new URL("../manifest.json", import.meta.url), "utf8"));
  const content = readFileSync(new URL("../content.js", import.meta.url), "utf8");
  const background = readFileSync(new URL("../background.js", import.meta.url), "utf8");
  assert.deepEqual(manifest.content_scripts[0].js, ["gemini.js", "content.js"]);
  assert.equal(manifest.permissions.includes("alarms"), true);
  assert.equal(manifest.permissions.includes("scripting"), true);
  assert.doesNotMatch(content, /attachShadow|webegg-connector-host|WebEggTranscript|send-prompt/);
  assert.match(content, /webegg:bridge-next/);
  assert.match(content, /webegg:bridge-event/);
  assert.match(content, /webegg:poke/);
  assert.match(background, /webegg-keepalive/);
  assert.match(background, /chrome\.scripting\.executeScript/);
});
