/* ══════════════ API 客户端 ══════════════ */
const API = '/api';
async function apiReq(path, opts) {
  const r = await fetch(API + path, opts);
  const j = await r.json().catch(() => ({}));
  if (!r.ok || j.code) throw new Error(j.msg || ('HTTP ' + r.status));
  return j.data;
}
function apiGet(p) { return apiReq(p); }
function apiPost(p, body) { return apiReq(p, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(body) }); }
function apiDelete(p) { return apiReq(p, { method:'DELETE' }); }
function modelToPayload(m) {
  return { id: m.profile.profileId, profileIndex: toNum(m.profile.profileIndex, 0), name: m.profile.name || '', profile: m.profile, properties: m.properties };
}
async function loadModels() {
  try {
    const rows = await apiGet('/models');
    state.models = (rows || []).map(r => ({ id: (r.profile && r.profile.profileId) || r.id, profile: r.profile || {}, properties: r.properties || [] }));
  } catch (e) { state.models = []; }
}
async function loadHardware() {
  try {
    const hw = await apiGet('/hardware');
    state.hardware = (hw && typeof hw === 'object') ? hw : DEFAULT_HARDWARE;
  } catch (e) { state.hardware = DEFAULT_HARDWARE; }
}
/* ── 链路持久化（/api/channels）── */
function channelFromRow(r) {
  return { id: toNum(r.id, 0), name: r.name || '', type: r.type || '',
    frameInterval: r.config && r.config.frameInterval, reconnectRetries: r.config && r.config.reconnectRetries, resendRetries: r.config && r.config.resendRetries,
    serialName: hwKey('Serial', r.config && r.config.serialName), baudRate: r.config && r.config.baudRate, dataBits: r.config && r.config.dataBits, parity: r.config && r.config.parity, stopBits: r.config && r.config.stopBits,
    nicName: hwKey('Ethernet', r.config && r.config.nicName), deviceIp: r.config && r.config.deviceIp, devicePort: r.config && r.config.devicePort,
    canName: hwKey('CAN', r.config && r.config.canName), canBaud: r.config && r.config.canBaud,
    devices: (r.devices || []).map((d, i) => ({ index: i, commNo: String(d.commNo), modelId: String(d.modelId) })) };
}
function channelToPayload(c) {
  const config = buildChannelConfig(c);
  return { id: toNum(c.id, 0), name: c.name || '', type: c.type || '', config: config, devices: config.devices || [] };
}
async function loadChannels() {
  try {
    const rows = await apiGet('/channels');
    state.channels = (rows || []).map(channelFromRow);
  } catch (e) { state.channels = []; }
}

/* ══════════════ Download helpers ══════════════ */
function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url; a.download = filename;
  document.body.appendChild(a); a.click(); a.remove();
  URL.revokeObjectURL(url);
}
function downloadJson(obj, filename) { downloadBlob(new Blob([JSON.stringify(obj, null, 2)], { type: 'application/json' }), filename); }

