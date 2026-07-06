/* ══════════════ 实时数据 · Realtime ══════════════ */
const rtValues = {};
function channelDevices(ch) {
  if (!ch) return [];
  return (ch.devices || []).map(d => {
    const m = state.models.find(x => String(x.id) === String(d.modelId));
    return m ? { commNo: d.commNo, model: m } : null;
  }).filter(Boolean);
}
function channelDeviceModel(channelSelId, deviceSelId) {
  const ch = state.channels.find(c => String(c.id) === document.getElementById(channelSelId).value);
  if (!ch) return null;
  const commNo = document.getElementById(deviceSelId).value;
  const d = (ch.devices || []).find(x => String(x.commNo) === String(commNo));
  return d ? state.models.find(m => String(m.id) === String(d.modelId)) : null;
}
function fillSelect(sel, items, getVal, getLabel, keepVal) {
  const prev = keepVal ? sel.value : '';
  sel.innerHTML = items.length ? items.map(it => `<option value="${escapeHtml(getVal(it))}">${escapeHtml(getLabel(it))}</option>`).join('')
    : '<option value="">无可用项</option>';
  if (prev && items.some(it => String(getVal(it)) === prev)) sel.value = prev;
}
function genMockValue(p) {
  const min = 0;
  const max = 100;
  switch (p.dataType) {
    case 'bool': return Math.random() > 0.5 ? 1 : 0;
    case 'enum': return Math.floor(Math.random() * 4);
    case 'bitmap': return Math.floor(Math.random() * 256);
    case 'int': return Math.floor(min + Math.random() * (max - min));
    case 'float': return Number((min + Math.random() * (max - min)).toFixed(2));
    case 'string': return 'OK';
    default: return Math.floor(Math.random() * 100);
  }
}
function renderRealtime() {
  fillSelect(document.getElementById('rt-channel'), state.channels, c => c.id, c => `通道${c.id} · ${c.name}`, true);
  onRtChannelChange();
}
function onRtChannelChange() {
  const ch = state.channels.find(c => String(c.id) === document.getElementById('rt-channel').value);
  fillSelect(document.getElementById('rt-device'), channelDevices(ch), d => d.commNo, d => `#${d.commNo} · ${(d.model.profile && d.model.profile.name) || '未命名设备'}`, true);
  renderRtTable();
}
function renderRtTable() {
  const m = channelDeviceModel('rt-channel', 'rt-device');
  const empty = document.getElementById('rt-empty'), wrap = document.getElementById('rt-wrap'), tb = document.getElementById('rt-tbody');
  if (!m || !m.properties.length) { wrap.classList.add('d-none'); empty.classList.remove('d-none'); empty.querySelector('div').textContent = m ? '该设备暂无属性' : '请选择通道与设备以查看实时属性数值。'; return; }
  empty.classList.add('d-none'); wrap.classList.remove('d-none');
  tb.innerHTML = m.properties.map(p => {
    const key = m.id + ':' + p.id;
    if (!(key in rtValues)) rtValues[key] = genMockValue(p);
    const writable = p.accessMode === 'w' || p.accessMode === 'rw';
    const setCell = writable
      ? `<input type="text" class="form-control form-control-sm rt-set-input" id="set-${escapeHtml(p.id)}" placeholder="设定值">
         <button class="btn btn-sm btn-primary ms-1" onclick="setRtValue('${escapeHtml(p.id)}')"><i class="bi bi-upload"></i> 下发</button>`
      : '<span class="text-muted">只读</span>';
    return `<tr>
      <td><code>${escapeHtml(p.id)}</code></td><td>${escapeHtml(p.name)}</td>
      <td class="fw-semibold" id="rtval-${escapeHtml(p.id)}">${escapeHtml(String(rtValues[key]))}</td>
      <td>${escapeHtml(p.unit || '—')}</td>
      <td><span class="badge badge-${escapeHtml(p.accessMode)}">${escapeHtml((p.accessMode||'').toUpperCase())}</span></td>
      <td class="text-nowrap">${setCell}</td>
    </tr>`;
  }).join('');
}
function setRtValue(propId) {
  const m = channelDeviceModel('rt-channel', 'rt-device'); if (!m) return;
  const v = document.getElementById('set-' + propId).value.trim();
  if (v === '') { alert('请输入设定值'); return; }
  rtValues[m.id + ':' + propId] = v;
  document.getElementById('rtval-' + propId).textContent = v;
  alert(`已下发：${propId} = ${v}`);
}

