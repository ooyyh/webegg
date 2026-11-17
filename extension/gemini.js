(() => {
  function findInput(doc = document) {
    const selectors = [
      'rich-textarea div.ql-editor[contenteditable="true"]',
      'div.ql-editor[contenteditable="true"]',
      'div[contenteditable="true"][role="textbox"]',
      'textarea[aria-label*="prompt" i]',
      'textarea[placeholder*="prompt" i]',
      'textarea',
    ];
    return firstMatch(doc, selectors);
  }

  function findSendButton(doc = document) {
    const selectors = [
      'button[data-testid="send-button"]',
      'button.send-button:not(.stop)',
      'button[aria-label*="Send message" i]',
      'button[aria-label*="发送" i]',
      'ms-run-button button',
    ];
    return firstMatch(doc, selectors);
  }

  function isGenerating(doc = document) {
    const selectors = [
      'button.send-button.stop',
      'button.stop',
      'button[aria-label*="Stop response" i]',
      'button[aria-label*="停止" i]',
    ];
    return Boolean(firstMatch(doc, selectors));
  }

  function isSendButtonReady(button) {
    return Boolean(
      button &&
      !button.disabled &&
      button.getAttribute("aria-disabled") !== "true" &&
      button.getAttribute("data-disabled") !== "true"
    );
  }

  function readInputValue(input) {
    if (!input) return "";
    if ("value" in input) return String(input.value || "");
    return String(input.innerText || input.textContent || "");
  }

  function setInputValue(input, value) {
    if (!input) return false;
    input.focus();

    if (input.isContentEditable) {
      const view = input.ownerDocument.defaultView;
      const selection = view.getSelection();
      const range = input.ownerDocument.createRange();
      range.selectNodeContents(input);
      selection.removeAllRanges();
      selection.addRange(range);

      let inserted = false;
      try {
        inserted = input.ownerDocument.execCommand("insertText", false, value);
      } catch {
        inserted = false;
      }

      if (!inserted || readInputValue(input).trim() !== value.trim()) {
        try {
          const transfer = new DataTransfer();
          transfer.setData("text/plain", value);
          input.dispatchEvent(new ClipboardEvent("paste", {
            clipboardData: transfer,
            bubbles: true,
            cancelable: true,
          }));
        } catch {
          // The final textContent + input event path below covers editors without DataTransfer.
        }
      }

      if (readInputValue(input).trim() !== value.trim()) {
        input.textContent = value;
        input.dispatchEvent(new InputEvent("input", {
          bubbles: true,
          inputType: "insertText",
          data: value,
        }));
      }
    } else {
      const view = input.ownerDocument.defaultView;
      const isTextarea = input.tagName.toLowerCase() === "textarea";
      const prototype = isTextarea ? view.HTMLTextAreaElement.prototype : view.HTMLInputElement.prototype;
      const setter = Object.getOwnPropertyDescriptor(prototype, "value")?.set;
      if (input._valueTracker) input._valueTracker.setValue("");
      if (setter) setter.call(input, value);
      else input.value = value;
      input.dispatchEvent(new Event("input", { bubbles: true }));
      input.dispatchEvent(new Event("change", { bubbles: true }));
    }

    return readInputValue(input).trim() === value.trim();
  }

  function findLatestReply(doc = document) {
    const selectors = [
      "model-response",
      "[data-test-id='model-response']",
      "ms-chat-turn[data-turn-role='Model']",
      "message-content",
      ".model-response-text",
    ];
    for (const selector of selectors) {
      const replies = doc.querySelectorAll(selector);
      if (replies.length) return replies[replies.length - 1];
    }
    return null;
  }

  function extractToolCall(reply) {
    if (!reply) return null;
    const blocks = Array.from(reply.querySelectorAll("pre")).reverse();
    for (const block of blocks) {
      const code = block.querySelector("code") || block;
      const raw = String(code.textContent || "").trim();
      const language = `${block.className || ""} ${code.className || ""}`.toLowerCase();
      let candidate = "";
      const fenced = raw.match(/^```webegg\s*([\s\S]*?)```$/i);
      if (fenced) candidate = fenced[1].trim();
      else if (/webegg/.test(language)) candidate = raw;
      else if (/^webegg\s*\n/i.test(raw)) candidate = raw.replace(/^webegg\s*\n/i, "").trim();
      else if (raw.startsWith("{")) candidate = raw;
      if (!candidate) continue;
      return parseToolCall(candidate, block);
    }

    const text = String(reply.innerText || reply.textContent || "");
    const matches = Array.from(text.matchAll(/```webegg\s*([\s\S]*?)```/gi));
    if (!matches.length) return null;
    return parseToolCall(matches[matches.length - 1][1].trim(), reply);
  }

  function parseToolCall(candidate, element) {
    try {
      const call = JSON.parse(candidate);
      if (!call || typeof call.tool !== "string" || !call.tool.startsWith("webegg_") ||
          !call.arguments || typeof call.arguments !== "object" || Array.isArray(call.arguments)) {
        return { invalid: true, element, raw: candidate };
      }
      return { tool: call.tool, arguments: call.arguments, element, raw: candidate };
    } catch {
      return { invalid: true, element, raw: candidate };
    }
  }

  function startNewChat(doc = document) {
    const button = firstMatch(doc, [
      "[data-test-id='new-chat-button'] a",
      "a[aria-label*='New chat' i]",
      "button[aria-label*='New chat' i]",
      "a[aria-label*='新对话' i]",
      "button[aria-label*='新对话' i]",
    ]);
    if (!button) return false;
    button.click();
    return true;
  }

  function hasCompletionMarker(text) {
    return /\[WebEgg:DONE\]/i.test(String(text || ""));
  }

  function needsContinuation(text, toolCount, taskText) {
    const completed = hasCompletionMarker(text);
    const requiresDepth = /分析|修复|实现|重构|检查|浏览|优化|添加|开发|排查|审阅|改造|设计|搭建/.test(String(taskText || ""));
    const tooShallow = requiresDepth && toolCount < 2;
    return !completed || tooShallow;
  }

  function stripCompletionMarker(text) {
    return String(text || "").replace(/\[WebEgg:DONE\]/gi, "").replace(/^\s+/, "").trim();
  }

  function getReplyText(reply) {
    if (!reply) return "";
    const clone = reply.cloneNode(true);
    clone.querySelectorAll("model-thoughts, [class*='thought'], .code-block-decoration, message-actions, button, mat-icon").forEach((node) => node.remove());
    clone.querySelectorAll("pre").forEach((block) => {
      const text = block.textContent || "";
      if (text.includes('"tool"') && text.includes('webegg_')) block.remove();
    });
    return String(clone.textContent || "").trim();
  }

  function firstMatch(doc, selectors) {
    for (const selector of selectors) {
      const element = doc.querySelector(selector);
      if (element) return element;
    }
    return null;
  }

  globalThis.WebEggGemini = {
    findInput,
    findSendButton,
    findLatestReply,
    extractToolCall,
    getReplyText,
    hasCompletionMarker,
    needsContinuation,
    stripCompletionMarker,
    startNewChat,
    isGenerating,
    isSendButtonReady,
    readInputValue,
    setInputValue,
  };
})();
