(() => {
  if (globalThis.__webeggProviderAdapter) return;
  globalThis.__webeggProviderAdapter = true;

  let active = null;
  let polling = false;

  async function emit(type, payload = {}) {
    if (!active) return;
    const response = await chrome.runtime.sendMessage({
      type: "webegg:bridge-event",
      event: { type, commandId: active.command.id, ...payload },
    });
    if (!response?.ok) throw new Error(response?.error?.message || "无法同步本地对话");
  }

  async function poll() {
    if (polling) return;
    polling = true;
    try {
      const response = await chrome.runtime.sendMessage({ type: "webegg:bridge-next", provider: "gemini", activeCommandId: active?.command.id || "" });
      if (response?.ok && response.command && !active) {
        void startCommand(response.command, response.toolCallLimit || 0);
      }
    } catch {
      // The local app keeps commands queued while the browser is reconnecting.
    } finally {
      polling = false;
    }
  }

  async function startCommand(command, toolCallLimit = 0) {
    active = {
      command,
      toolCallLimit,
      baseline: globalThis.WebEggGemini.findLatestReply(),
      reply: null,
      lastText: "",
      lastChange: Date.now(),
      startedAt: Date.now(),
      url: location.href,
      watching: false,
      toolIndex: 0,
      processed: new WeakSet(),
      autoContinues: 0,
      taskText: command.prompt || "",
    };
    try {
      await emit("ack");
      if (command.newSession) {
        const started = globalThis.WebEggGemini.startNewChat();
        if (!started && !/^\/app\/?$/.test(location.pathname)) {
          throw new Error("请在 Gemini 中新建一个空白对话后，再从本地发送");
        }
        await new Promise((resolve) => setTimeout(resolve, 1200));
        active.url = location.href;
        active.baseline = globalThis.WebEggGemini.findLatestReply();
      }
      const sections = [];
      if (command.initialize || command.newSession) {
        const protocol = await chrome.runtime.sendMessage({ type: "webegg:build-prompt" });
        if (!protocol?.ok) throw new Error(protocol?.error?.message || "无法同步工具协议");
        sections.push(protocol.prompt);
      }
      if (command.history) sections.push(`【本地会话历史】\n${command.history}`);
      if (command.prompt) {
        sections.push(`【用户任务】\n${command.prompt}`);
      } else {
        sections.push(`【初始化任务】\n先调用 webegg_project_list 确认项目环境；只有一个项目时继续调用 webegg_directory_list 查看根目录。完成后以 [WebEgg:DONE] 简短汇报环境，等待用户新的任务，不要自行修改项目。`);
      }
      await sendToGemini(sections.join("\n\n"));
      scheduleWatch();
    } catch (error) {
      await failCommand(error);
    }
  }

  async function sendToGemini(text) {
    if (!active) return;
    if (globalThis.WebEggGemini.isGenerating()) throw new Error("Gemini 正在生成其他回复，请稍后重试");
    const input = await waitFor(() => globalThis.WebEggGemini.findInput(), 8000);
    if (!input) throw new Error("未找到 Gemini 输入框，请打开可对话的页面");
    const draft = globalThis.WebEggGemini.readInputValue(input).trim();
    if (draft && draft !== text.trim()) throw new Error("Gemini 输入框已有手动草稿，请先处理后再发送本地消息");
    if (!globalThis.WebEggGemini.setInputValue(input, text)) throw new Error("无法写入 Gemini 输入框");
    const button = await waitFor(() => {
      const candidate = globalThis.WebEggGemini.findSendButton();
      return globalThis.WebEggGemini.isSendButtonReady(candidate) ? candidate : null;
    }, 6000);
    if (!button) throw new Error("Gemini 发送按钮不可用");
    active.baseline = globalThis.WebEggGemini.findLatestReply();
    active.reply = null;
    active.lastText = "";
    button.click();
  }

  async function watch() {
    if (!active || active.watching) return;
    active.watching = true;
    try {
      if (location.href !== active.url) {
        if (active.command.newSession && !active.lastText) active.url = location.href;
        else throw new Error("Gemini 页面会话已切换，请从本地重新发送");
      }
      if (Date.now() - active.startedAt > 30 * 60 * 1000) throw new Error("网页对话超过 30 分钟，已停止等待");
      const reply = globalThis.WebEggGemini.findLatestReply();
      if (!reply || reply === active.baseline) return;
      if (active.reply && active.reply !== reply) {
        active.lastText = "";
        active.lastChange = Date.now();
      }
      active.reply = reply;
      const generating = globalThis.WebEggGemini.isGenerating();
      const call = globalThis.WebEggGemini.extractToolCall(reply);
      const stableFor = Date.now() - active.lastChange;

      if (call && !call.invalid && !active.processed.has(call.element) && !generating && stableFor > 500) {
        active.processed.add(call.element);
        await executeTool(call);
        return;
      }

      const text = globalThis.WebEggGemini.getReplyText(reply);
      if (text !== active.lastText) {
        active.lastText = text;
        active.lastChange = Date.now();
        if (text) await emit("assistant.delta", { text });
        return;
      }
      if (!generating && !call && text && stableFor > 1600) {
        const completed = globalThis.WebEggGemini.hasCompletionMarker(text);
        const shouldContinue = globalThis.WebEggGemini.needsContinuation(text, active.toolIndex, active.taskText);
        if (shouldContinue && active.autoContinues < 3) {
          active.autoContinues++;
          const tooShallow = completed && /分析|修复|实现|重构|检查|浏览|优化|添加|开发|排查|审阅|改造|设计|搭建/.test(active.taskText) && active.toolIndex < 2;
          const reason = tooShallow ? "复杂任务只完成了一次工具调用" : "任务尚未达到完成标准";
          const continuation = `【WebEgg 自动续行】\n${reason}。请继续检查相关文件、执行必要操作并验证结果；不要总结，不要输出 [WebEgg:DONE]，直接继续下一个 webegg 工具调用。自动续行 ${active.autoContinues}/3。`;
          await sendToGemini(continuation);
          return;
        }
        let finalText = globalThis.WebEggGemini.stripCompletionMarker(text);
        if (!completed) finalText += "\n\n> 自动续行达到上限，任务可能尚未完全完成。";
        if (!finalText) finalText = "任务已完成。";
        await emit("assistant.done", { text: finalText });
        active = null;
        setTimeout(poll, 300);
      }
    } catch (error) {
      await failCommand(error);
    } finally {
      if (active) active.watching = false;
    }
  }

  async function executeTool(call) {
    if (active.toolCallLimit > 0 && active.toolIndex >= active.toolCallLimit) {
      throw new Error(`本次任务已达到桌面端设置的 ${active.toolCallLimit} 次工具调用上限`);
    }
    active.toolIndex++;
    const callID = `tool_${active.command.id}_${active.toolIndex}`;
    await emit("tool.started", { callId: callID, tool: call.tool, arguments: call.arguments });
    const result = await chrome.runtime.sendMessage({
      type: "webegg:execute-tool", name: call.tool, arguments: call.arguments,
    });
    if (!result || typeof result.ok !== "boolean") throw new Error("工具桥接返回格式错误");
    await emit("tool.finished", { callId: callID, result });
    const feedback = `【WebEgg 工具结果】\n${JSON.stringify({ tool: call.tool, ok: result.ok, result: result.result, error: result.error })}`;
    await sendToGemini(feedback);
    active.baseline = active.reply;
    active.reply = null;
    active.lastText = "";
    active.lastChange = Date.now();
  }

  async function failCommand(error) {
    if (!active) return;
    const message = error?.message || String(error || "网页对话失败");
    try { await emit("error", { message }); } catch { /* Keep the command visible as failed locally. */ }
    active = null;
    setTimeout(poll, 1000);
  }

  function scheduleWatch() {
    if (!active) return;
    setTimeout(() => {
      void watch().finally(() => {
        if (active) setTimeout(scheduleWatch, 350);
      });
    }, 200);
  }

  async function waitFor(getValue, timeoutMs) {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      const value = getValue();
      if (value) return value;
      await new Promise((resolve) => setTimeout(resolve, 150));
    }
    return null;
  }

  function onRuntimeMessage(message) {
    if (message?.type === "webegg:poke") {
      void poll();
    }
    return false;
  }
  chrome.runtime.onMessage.addListener(onRuntimeMessage);

  const observer = new MutationObserver(() => {
    if (active && !active.watching) void watch();
  });
  observer.observe(document.documentElement, { childList: true, subtree: true, characterData: true });
  document.addEventListener("visibilitychange", poll);
  window.addEventListener("pagehide", () => {
    observer.disconnect();
    chrome.runtime.onMessage.removeListener(onRuntimeMessage);
  }, { once: true });
  setInterval(poll, 1200);
  poll();
})();
