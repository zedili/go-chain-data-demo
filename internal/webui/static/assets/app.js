"use strict";

const TRANSFER_TOPIC = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef";
const HISTORY_KEY = "chainscope.queryHistory.v1";
const CHAIN_NAMES = {
  "1": "Ethereum",
  "10": "Optimism",
  "56": "BNB Chain",
  "137": "Polygon",
  "8453": "Base",
  "42161": "Arbitrum One",
  "11155111": "Sepolia",
};

const elements = {
  sidebarStatus: document.querySelector("#sidebar-status"),
  sidebarStatusDot: document.querySelector("#sidebar-status-dot"),
  networkName: document.querySelector("#network-name"),
  metricNetwork: document.querySelector("#metric-network"),
  metricChainId: document.querySelector("#metric-chain-id"),
  metricHeight: document.querySelector("#metric-height"),
  metricBlockTime: document.querySelector("#metric-block-time"),
  metricTransactions: document.querySelector("#metric-transactions"),
  metricLatency: document.querySelector("#metric-latency"),
  metricHealth: document.querySelector("#metric-health"),
  latestHash: document.querySelector("#latest-hash"),
  latestParent: document.querySelector("#latest-parent"),
  latestGas: document.querySelector("#latest-gas"),
  latestFee: document.querySelector("#latest-fee"),
  resultTitle: document.querySelector("#result-title"),
  resultMeta: document.querySelector("#result-meta"),
  resultEndpoint: document.querySelector("#result-endpoint"),
  resultOutput: document.querySelector("#result-output"),
  copyResult: document.querySelector("#copy-result"),
  history: document.querySelector("#query-history"),
  toast: document.querySelector("#toast"),
};

let latestBlock = null;
let currentResult = null;
let queryHistory = loadHistory();

async function fetchJSON(url) {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), 15000);
  const started = performance.now();
  try {
    const response = await fetch(url, {
      headers: { Accept: "application/json" },
      signal: controller.signal,
    });
    const duration = Math.round(performance.now() - started);
    let data;
    try {
      data = await response.json();
    } catch {
      data = { error: { code: "invalid_response", message: "服务端没有返回有效 JSON" } };
    }
    if (!response.ok) {
      const error = new Error(data?.error?.message || `HTTP ${response.status}`);
      error.status = response.status;
      error.payload = data;
      error.duration = duration;
      throw error;
    }
    return { data, duration };
  } catch (error) {
    if (error.name === "AbortError") {
      throw new Error("请求超过 15 秒，已自动取消");
    }
    throw error;
  } finally {
    window.clearTimeout(timeout);
  }
}

async function loadOverview(showFailure = false) {
  setConnection("loading", "正在连接");
  const started = performance.now();
  try {
    const [health, network, block] = await Promise.all([
      fetchJSON("/healthz"),
      fetchJSON("/api/v1/network"),
      fetchJSON("/api/v1/blocks/latest"),
    ]);
    const latency = Math.round(performance.now() - started);
    latestBlock = block.data;
    const chainId = String(network.data.chainId);
    const chainName = CHAIN_NAMES[chainId] || `EVM Chain ${chainId}`;

    elements.networkName.textContent = chainName;
    elements.metricNetwork.textContent = chainName;
    elements.metricChainId.textContent = `Chain ID ${chainId}`;
    elements.metricHeight.textContent = formatNumber(network.data.latestBlockNumber);
    elements.metricBlockTime.textContent = formatDate(block.data.timestampISO8601);
    elements.metricTransactions.textContent = formatNumber(block.data.transactionCount);
    elements.metricLatency.textContent = String(latency);
    elements.metricHealth.textContent = health.data.status === "ok" ? "节点连接正常" : "状态未知";
    elements.latestHash.textContent = block.data.hash || "—";
    elements.latestParent.textContent = block.data.parentHash || "—";
    elements.latestGas.textContent = `${formatNumber(block.data.gasUsed)} / ${formatNumber(block.data.gasLimit)}`;
    elements.latestFee.textContent = block.data.baseFeePerGas ? `${block.data.baseFeePerGas} wei` : "—";
    setConnection("online", "连接正常");
  } catch (error) {
    setConnection("offline", "连接失败");
    elements.metricHealth.textContent = error.message;
    elements.metricLatency.textContent = "—";
    if (showFailure) renderError(error, "概览刷新失败", "/api/v1/network");
  }
}

async function runQuery(url, title, button = null, record = true) {
  setButtonLoading(button, true);
  elements.resultTitle.textContent = title;
  elements.resultEndpoint.textContent = url;
  elements.resultMeta.textContent = "REQUESTING";
  elements.resultOutput.className = "loading-output";
  elements.resultOutput.innerHTML = "正在读取链上数据…";
  document.querySelector("#result-section").scrollIntoView({ behavior: "smooth", block: "start" });

  try {
    const { data, duration } = await fetchJSON(url);
    currentResult = data;
    elements.resultOutput.className = "";
    elements.resultOutput.innerHTML = syntaxHighlight(data);
    elements.resultMeta.textContent = `200 OK · ${duration} MS`;
    elements.copyResult.disabled = false;
    setConnection("online", "连接正常");
    if (record) addHistory({ title, url, status: 200, duration, at: Date.now() });
  } catch (error) {
    renderError(error, title, url);
    if (record) addHistory({ title, url, status: error.status || "ERR", duration: error.duration || 0, at: Date.now() });
  } finally {
    setButtonLoading(button, false);
  }
}

function renderError(error, title, url) {
  currentResult = error.payload || { error: { code: "request_failed", message: error.message } };
  elements.resultTitle.textContent = title;
  elements.resultEndpoint.textContent = url;
  elements.resultMeta.textContent = `${error.status || "NETWORK ERROR"}${error.duration ? ` · ${error.duration} MS` : ""}`;
  elements.resultOutput.className = "error-output";
  elements.resultOutput.textContent = JSON.stringify(currentResult, null, 2);
  elements.copyResult.disabled = false;
  if (!error.status || error.status >= 500) setConnection("offline", "RPC 异常");
}

function syntaxHighlight(value) {
  const json = escapeHTML(JSON.stringify(value, null, 2));
  return json.replace(/(&quot;(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\&])*&quot;)(\s*:)?|\b(true|false)\b|\b(null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g, (match, string, keyMarker, bool, nil, number) => {
    if (keyMarker) return `<span class="json-key">${string}</span>${keyMarker}`;
    if (string) return `<span class="json-string">${string}</span>`;
    if (bool) return `<span class="json-boolean">${bool}</span>`;
    if (nil) return `<span class="json-null">${nil}</span>`;
    if (number) return `<span class="json-number">${number}</span>`;
    return match;
  });
}

function escapeHTML(value) {
  return value.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}

function setConnection(state, label) {
  elements.sidebarStatus.textContent = label;
  elements.sidebarStatusDot.classList.remove("online", "offline");
  document.querySelectorAll(".network-pill .status-dot").forEach((dot) => dot.classList.remove("online", "offline"));
  if (state === "online" || state === "offline") {
    elements.sidebarStatusDot.classList.add(state);
    document.querySelectorAll(".network-pill .status-dot").forEach((dot) => dot.classList.add(state));
  }
}

function setButtonLoading(button, loading) {
  if (!button) return;
  if (loading) {
    button.dataset.label = button.textContent;
    button.textContent = "查询中…";
    button.disabled = true;
  } else {
    button.textContent = button.dataset.label || button.textContent;
    button.disabled = false;
  }
}

function formatNumber(value) {
  if (value === null || value === undefined) return "—";
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatDate(value) {
  if (!value) return "等待同步";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
  }).format(date);
}

function isAddress(value) { return /^0x[0-9a-fA-F]{40}$/.test(value); }
function isHash(value) { return /^0x[0-9a-fA-F]{64}$/.test(value); }
function isDecimal(value) { return /^\d+$/.test(value); }

function bindForm(selector, handler) {
  document.querySelector(selector).addEventListener("submit", (event) => {
    event.preventDefault();
    handler(event.currentTarget);
  });
}

bindForm("#block-form", (form) => {
  const value = document.querySelector("#block-number").value.trim();
  if (value && !isDecimal(value)) return showToast("区块高度必须是十进制整数");
  runQuery(value ? `/api/v1/blocks/${value}` : "/api/v1/blocks/latest", value ? `区块 #${value}` : "最新区块", form.querySelector("button"));
});

bindForm("#transaction-form", (form) => {
  const hash = document.querySelector("#transaction-hash").value.trim();
  if (!isHash(hash)) return showToast("请输入完整的 32 字节交易哈希");
  runQuery(`/api/v1/transactions/${hash}`, "交易详情", form.querySelector("button"));
});

bindForm("#balance-form", (form) => {
  const address = document.querySelector("#balance-address").value.trim();
  const block = document.querySelector("#balance-block").value.trim() || "latest";
  if (!isAddress(address)) return showToast("请输入完整的 20 字节账户地址");
  if (block !== "latest" && !isDecimal(block)) return showToast("区块必须是 latest 或十进制整数");
  runQuery(`/api/v1/accounts/${address}/balance?block=${encodeURIComponent(block)}`, "账户余额", form.querySelector("button"));
});

bindForm("#logs-form", (form) => {
  const address = document.querySelector("#logs-address").value.trim();
  const from = document.querySelector("#logs-from").value.trim();
  const to = document.querySelector("#logs-to").value.trim();
  const topic = document.querySelector("#logs-topic").value.trim();
  if (!isAddress(address)) return showToast("请输入完整的合约地址");
  if (!isDecimal(from) || !isDecimal(to)) return showToast("请填写有效的起始和结束区块");
  if (BigInt(to) < BigInt(from)) return showToast("结束区块不能小于起始区块");
  if (BigInt(to) - BigInt(from) > 5000n) return showToast("日志查询最大跨度为 5000 个区块");
  if (topic && !isHash(topic)) return showToast("Topic 0 必须是完整的 32 字节哈希");
  const params = new URLSearchParams({ address, fromBlock: from, toBlock: to });
  if (topic) params.set("topic0", topic);
  runQuery(`/api/v1/logs?${params}`, "合约事件日志", form.querySelector("button"));
});

document.querySelector("#refresh-overview").addEventListener("click", () => loadOverview(true));
document.querySelector("#inspect-latest").addEventListener("click", () => {
  if (latestBlock) {
    currentResult = latestBlock;
    elements.resultTitle.textContent = "最新区块";
    elements.resultEndpoint.textContent = "/api/v1/blocks/latest";
    elements.resultMeta.textContent = "CACHED OVERVIEW";
    elements.resultOutput.className = "";
    elements.resultOutput.innerHTML = syntaxHighlight(latestBlock);
    elements.copyResult.disabled = false;
    document.querySelector("#result-section").scrollIntoView({ behavior: "smooth" });
  } else {
    runQuery("/api/v1/blocks/latest", "最新区块");
  }
});
document.querySelector("#fill-transfer-topic").addEventListener("click", () => {
  document.querySelector("#logs-topic").value = TRANSFER_TOPIC;
  showToast("已填入 ERC-20 / ERC-721 Transfer Topic");
});
elements.copyResult.addEventListener("click", async () => {
  if (!currentResult) return;
  try {
    await navigator.clipboard.writeText(JSON.stringify(currentResult, null, 2));
    showToast("JSON 已复制");
  } catch {
    showToast("复制失败，请手动选择结果文本");
  }
});
document.querySelector("#clear-history").addEventListener("click", () => {
  queryHistory = [];
  saveHistory();
  renderHistory();
});

function addHistory(item) {
  queryHistory = [item, ...queryHistory.filter((entry) => entry.url !== item.url)].slice(0, 8);
  saveHistory();
  renderHistory();
}

function loadHistory() {
  try {
    const parsed = JSON.parse(localStorage.getItem(HISTORY_KEY) || "[]");
    return Array.isArray(parsed) ? parsed.slice(0, 8) : [];
  } catch {
    return [];
  }
}

function saveHistory() {
  try { localStorage.setItem(HISTORY_KEY, JSON.stringify(queryHistory)); } catch { /* storage may be unavailable */ }
}

function renderHistory() {
  if (!queryHistory.length) {
    elements.history.innerHTML = '<li class="empty-history">暂无查询记录</li>';
    return;
  }
  elements.history.innerHTML = "";
  queryHistory.forEach((item) => {
    const row = document.createElement("li");
    const button = document.createElement("button");
    button.type = "button";
    const title = document.createElement("strong");
    title.textContent = item.title;
    const meta = document.createElement("small");
    meta.textContent = `${item.status} · ${item.duration || 0} ms · ${new Date(item.at).toLocaleTimeString("zh-CN", { hour12: false })}`;
    button.append(title, meta);
    button.addEventListener("click", () => runQuery(item.url, item.title));
    row.append(button);
    elements.history.append(row);
  });
}

let toastTimer;
function showToast(message) {
  elements.toast.textContent = message;
  elements.toast.classList.add("show");
  window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => elements.toast.classList.remove("show"), 2200);
}

const sections = [...document.querySelectorAll("main section[id]")];
const navLinks = [...document.querySelectorAll(".nav-link")];
if ("IntersectionObserver" in window) {
  const observer = new IntersectionObserver((entries) => {
    const visible = entries.filter((entry) => entry.isIntersecting).sort((a, b) => b.intersectionRatio - a.intersectionRatio)[0];
    if (!visible) return;
    navLinks.forEach((link) => link.classList.toggle("active", link.getAttribute("href") === `#${visible.target.id}`));
  }, { rootMargin: "-25% 0px -60%", threshold: [0, .2, .6] });
  sections.forEach((section) => observer.observe(section));
}

renderHistory();
loadOverview();
