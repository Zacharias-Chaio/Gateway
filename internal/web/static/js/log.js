/* ══════════════ 日志监控 · Communication monitor ══════════════ */
let logEvents = [];
let logNextSeq = 0;
let logTimer = null;
let logLoading = false;

function renderLogSelectors() {
  fillSelect(document.getElementById('log-channel'), state.channels, c => c.id, c => `通道${c.id} · ${c.name}`, true);
  onLogChannelChange();
  logStartPolling();
}

function onLogChannelChange() {
  const ch = state.channels.find(c => String(c.id) === document.getElementById('log-channel').value);
  const devices = [{ index: -1, commNo: '', model: null }].concat(channelDevices(ch));
  fillSelect(document.getElementById('log-device'), devices,
    d => d.index, d => d.index < 0 ? '全部设备' : `#${d.commNo} · ${(d.model.profile && d.model.profile.name) || '未命名设备'}`, false);
  resetCommLog();
}

function onLogDeviceChange() {
  resetCommLog();
}

function resetCommLog() {
  logEvents = [];
  logNextSeq = 0;
  renderCommLog();
  fetchCommLog(true);
}

function logStartPolling() {
  logStopPolling();
  logTimer = setInterval(() => fetchCommLog(false), 2000);
}

function logStopPolling() {
  if (logTimer) { clearInterval(logTimer); logTimer = null; }
}

function logSelection() {
  const channel = document.getElementById('log-channel');
  const device = document.getElementById('log-device');
  if (!channel || !device || !channel.value) return null;
  return { channelId: toNum(channel.value, 0), deviceIndex: toNum(device.value, -1) };
}

async function fetchCommLog(force) {
  const sel = logSelection();
  if (!sel || !sel.channelId || logLoading) return;
  logLoading = true;
  const selectionKey = sel.channelId + '/' + sel.deviceIndex;
  try {
    let url = '/comm-monitor?channelId=' + encodeURIComponent(sel.channelId) + '&limit=200';
    if (sel.deviceIndex >= 0) url += '&deviceIndex=' + encodeURIComponent(sel.deviceIndex);
    if (!force && logNextSeq) url += '&afterSeq=' + encodeURIComponent(logNextSeq);
    const data = await apiGet(url);
    const active = logSelection();
    if (!active || active.channelId + '/' + active.deviceIndex !== selectionKey) return;
    if (force) logEvents = data.events || [];
    else logEvents = logEvents.concat(data.events || []);
    if (logEvents.length > 200) logEvents = logEvents.slice(-200);
    logNextSeq = data.nextSeq || 0;
    renderCommLog(data.stats);
  } catch (e) {
    renderCommLog(null, '暂无通讯监控数据（链路未运行或尚未建立连接）。');
  } finally {
    logLoading = false;
  }
}

function renderCommLog(stats, emptyMessage) {
  const errorRate = document.getElementById('log-err');
  if (errorRate) errorRate.textContent = stats ? Number(stats.errorRate || 0).toFixed(2) + '%' : '—';
  const consoleEl = document.getElementById('log-console');
  if (!consoleEl) return;
  if (!logEvents.length) {
    consoleEl.textContent = emptyMessage || '等待通讯报文…';
    return;
  }
  consoleEl.textContent = logEvents.map(formatCommEvent).join('\n');
  consoleEl.scrollTop = consoleEl.scrollHeight;
}

function formatCommEvent(event) {
  const time = event.time ? formatLogTime(event.time) : '—';
  const device = '#' + event.unitId;
  const operation = event.operation === 'write' ? '写入' : '读取';
  if (event.direction === 'ERR') return `${time} ${device} ERR ${operation} 第${event.attempt}次: ${event.error || '通讯失败'}`;
  return `${time} ${device} ${event.direction}: ${event.hex || ''}`;
}

function formatLogTime(value) {
  const date = new Date(value);
  const pad = (number, width = 2) => String(number).padStart(width, '0');
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}.${pad(date.getMilliseconds(), 3)}`;
}
