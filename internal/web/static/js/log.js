/* ══════════════ 日志监控 · Log ══════════════ */
function renderLogSelectors() {
  fillSelect(document.getElementById('log-channel'), state.channels, c => c.id, c => `通道${c.id} · ${c.name}`, true);
  onLogChannelChange();
}
function onLogChannelChange() {
  const ch = state.channels.find(c => String(c.id) === document.getElementById('log-channel').value);
  fillSelect(document.getElementById('log-device'), channelDevices(ch), d => d.commNo, d => `#${d.commNo} · ${(d.model.profile && d.model.profile.name) || '未命名设备'}`, true);
  renderLog();
}
function randHex(n) { return Array.from({length:n}, () => Math.floor(Math.random()*256).toString(16).padStart(2,'0').toUpperCase()).join(' '); }
function nowTs() { return new Date().toLocaleTimeString('zh-CN', { hour12:false }) + '.' + String(Date.now()%1000).padStart(3,'0'); }
function renderLog() {
  const err = (Math.random() * 0.5).toFixed(2);
  document.getElementById('log-err').textContent = err + '%';
  const m = channelDeviceModel('log-channel', 'log-device');
  const con = document.getElementById('log-console');
  if (!m) { con.innerHTML = '<div class="text-muted">请选择链路与设备以查看通讯日志。</div>'; return; }
  const lines = [];
  for (let i = 0; i < 12; i++) {
    lines.push(`<div><span class="log-ts">${nowTs()}</span><span class="log-tx">TX:</span> ${randHex(8)}</div>`);
    lines.push(`<div><span class="log-ts">${nowTs()}</span><span class="log-rx">RX:</span> ${randHex(2 + Math.floor(Math.random()*12))}</div>`);
  }
  con.innerHTML = lines.join('');
  con.scrollTop = con.scrollHeight;
}
