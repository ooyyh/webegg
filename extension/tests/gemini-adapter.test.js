import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

function loadBrowserScripts() {
  const context = vm.createContext({ console, Event });
  vm.runInContext(readFileSync(new URL("../prompt.js", import.meta.url), "utf8"), context);
  vm.runInContext(readFileSync(new URL("../gemini.js", import.meta.url), "utf8"), context);
  return context;
}

test("prompt embeds live tool schemas and strict call format", () => {
  const context = loadBrowserScripts();
  const prompt = context.WebEggBuildPrompt([{
    name: "webegg_file_read",
    description: "Read a local file",
    inputSchema: { type: "object", properties: { projectId: { type: "string" } } },
  }]);
  assert.match(prompt, /webegg_file_read/);
  assert.match(prompt, /inputSchema/);
  assert.match(prompt, /每次只输出一个 webegg 代码块/);
  assert.match(prompt, /收到工具结果后，必须立刻规划并输出下一次 webegg 工具调用/);
  assert.match(prompt, /禁止仅凭记忆、文件名或常识猜测项目内容/);
  assert.match(prompt, /expectedHash/);
  assert.doesNotMatch(prompt, /尚未开放任何项目查询/);
});

test("completion marker is detected and removed from local assistant text", () => {
  const { WebEggGemini } = loadBrowserScripts();
  assert.equal(WebEggGemini.hasCompletionMarker("[WebEgg:DONE]\n完成"), true);
  assert.equal(WebEggGemini.hasCompletionMarker("还没完成"), false);
  assert.equal(WebEggGemini.stripCompletionMarker("[WebEgg:DONE]\n完成"), "完成");
});

test("prompt requires iterative tool use and an explicit completion marker", () => {
  const context = loadBrowserScripts();
  const prompt = context.WebEggBuildPrompt([{ name: "webegg_file_read", description: "read", inputSchema: { type: "object" } }]);
  assert.match(prompt, /一次工具调用几乎不可能完成复杂任务/);
  assert.match(prompt, /输出 \[WebEgg:DONE\] 前自检/);
  assert.match(prompt, /不能在第一次读取后就停下/);
});

test("continuation rules keep multi-step tasks from stopping after one tool", () => {
  const { WebEggGemini } = loadBrowserScripts();
  assert.equal(WebEggGemini.needsContinuation("普通总结", 1, "读取 README"), true);
  assert.equal(WebEggGemini.needsContinuation("[WebEgg:DONE] 初步结论", 1, "分析项目结构"), true);
  assert.equal(WebEggGemini.needsContinuation("[WebEgg:DONE] 完整结论", 2, "分析项目结构"), false);
  assert.equal(WebEggGemini.needsContinuation("[WebEgg:DONE] 文件内容如下", 1, "读取 README.md"), false);
});

test("Gemini adapter finds current rich text input and send button", () => {
  const { WebEggGemini } = loadBrowserScripts();
  const input = { isContentEditable: true };
  const send = { disabled: false, getAttribute: () => null };
  const selectors = new Map([
    ['rich-textarea div.ql-editor[contenteditable="true"]', input],
    ['button[data-testid="send-button"]', send],
  ]);
  const doc = { querySelector: (selector) => selectors.get(selector) || null };
  assert.equal(WebEggGemini.findInput(doc), input);
  assert.equal(WebEggGemini.findSendButton(doc), send);
  assert.equal(WebEggGemini.isSendButtonReady(send), true);
});

test("Gemini adapter extracts only valid WebEgg tool calls", () => {
  const { WebEggGemini } = loadBrowserScripts();
  const code = {
    className: "",
    textContent: '{"tool":"webegg_file_read","arguments":{"projectId":"proj_1","path":"main.go"}}',
  };
  const block = { className: "", querySelector: () => code };
  const reply = { querySelectorAll: () => [block] };
  const call = WebEggGemini.extractToolCall(reply);
  assert.equal(call.tool, "webegg_file_read");
  assert.equal(call.arguments.path, "main.go");
  assert.equal(call.element, block);

  code.textContent = '{"tool":"webegg_file_read"';
  assert.equal(WebEggGemini.extractToolCall(reply).invalid, true);
});

test("send button readiness rejects disabled states", () => {
  const { WebEggGemini } = loadBrowserScripts();
  assert.equal(WebEggGemini.isSendButtonReady(null), false);
  assert.equal(WebEggGemini.isSendButtonReady({ disabled: true, getAttribute: () => null }), false);
  assert.equal(WebEggGemini.isSendButtonReady({
    disabled: false,
    getAttribute: (name) => name === "aria-disabled" ? "true" : null,
  }), false);
});

test("textarea writes use native setter and emit input events", () => {
  const { WebEggGemini } = loadBrowserScripts();
  const events = [];
  const prototype = {};
  Object.defineProperty(prototype, "value", {
    set(value) { this._value = value; },
    get() { return this._value || ""; },
  });
  const input = Object.create(prototype);
  Object.assign(input, {
    tagName: "TEXTAREA",
    ownerDocument: {
      defaultView: {
        HTMLTextAreaElement: { prototype },
        HTMLInputElement: { prototype: {} },
      },
    },
    focus() {},
    dispatchEvent(event) { events.push(event.type); },
  });

  assert.equal(WebEggGemini.setInputValue(input, "test prompt"), true);
  assert.equal(WebEggGemini.readInputValue(input), "test prompt");
  assert.deepEqual(events, ["input", "change"]);
});
