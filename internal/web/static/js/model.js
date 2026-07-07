/* ══════════════ Wizard navigation ══════════════ */
function goStep(n) {
  if (n < 1 || n > TOTAL_STEPS) return;
  if (n > 1 && currentStep === 1 && !validateProfile()) return;
  syncProfileFromForm();
  currentStep = n;
  renderStep();
}
function nextStep() { goStep(currentStep + 1); }
function prevStep() { goStep(currentStep - 1); }
function renderStep() {
  for (let i = 1; i <= TOTAL_STEPS; i++) {
    document.getElementById('step-' + i).classList.toggle('d-none', i !== currentStep);
    const si = document.getElementById('si-' + i);
    si.classList.toggle('active', i === currentStep);
    si.classList.toggle('completed', i < currentStep);
  }
  document.getElementById('btn-prev').disabled = currentStep === 1;
  document.getElementById('btn-next').disabled = currentStep === TOTAL_STEPS;
  document.getElementById('step-counter').textContent = `第 ${currentStep} 步 / 共 ${TOTAL_STEPS} 步`;
  if (currentStep === TOTAL_STEPS) renderPreview();
}

/* ══════════════ Step 1 · Profile ══════════════ */
function onInterfaceChange() { rebuildProtocolOptions(''); }
function rebuildProtocolOptions(selected) {
  const iface = document.getElementById('pf-interface').value;
  const sel = document.getElementById('pf-protocol');
  const protos = PROTOCOLS_BY_INTERFACE[iface] || [];
  sel.innerHTML = '<option value="">' + (protos.length ? '请选择协议类型' : '请先选择接口类型') + '</option>'
    + protos.map(p => `<option value="${escapeHtml(p)}">${escapeHtml(p)}</option>`).join('');
  if (selected && protos.includes(selected)) sel.value = selected;
}
function validateProfile() {
  const form = document.getElementById('form-profile');
  form.classList.add('was-validated');
  return form.checkValidity();
}
function syncProfileFromForm() {
  state.profile.profileIndex = val('pf-profileIndex');
  state.profile.profileId = val('pf-profileId');
  state.profile.name = val('pf-name');
  state.profile.manufacturer = val('pf-manufacturer');
  state.profile.description = val('pf-description');
  state.profile.deviceType = val('pf-deviceType');
  state.profile.deviceModel = val('pf-deviceModel');
  state.profile.ratedPower = val('pf-ratedPower');
  state.profile.interfaceType = val('pf-interface');
  state.profile.protocolType = val('pf-protocol');
  state.profile.protocolVersion = val('pf-version');
  state.profile.maxRegisterCount = val('pf-maxRegs');
}
function fillProfileForm() {
  setVal('pf-profileIndex', state.profile.profileIndex);
  setVal('pf-profileId', state.profile.profileId);
  setVal('pf-name', state.profile.name);
  setVal('pf-manufacturer', state.profile.manufacturer);
  setVal('pf-description', state.profile.description);
  setVal('pf-deviceType', state.profile.deviceType);
  setVal('pf-deviceModel', state.profile.deviceModel);
  setVal('pf-ratedPower', state.profile.ratedPower);
  setVal('pf-interface', state.profile.interfaceType);
  rebuildProtocolOptions(state.profile.protocolType);
  setVal('pf-version', state.profile.protocolVersion);
  setVal('pf-maxRegs', state.profile.maxRegisterCount ?? 125);
}

/* ══════════════ Project import / export (整体) ══════════════ */
/* ══════════════ Device model landing (multi-model) ══════════════ */
const IFACE_LABEL = { Serial:'串口', Network:'网络', CAN:'CAN' };
function ifaceLabel(v) { return IFACE_LABEL[v] || v || '—'; }
function emptyProfile() { return { profileIndex:'', profileId:'', name:'', manufacturer:'', description:'', deviceType:'', deviceModel:'', ratedPower:'', interfaceType:'', protocolType:'', protocolVersion:'', maxRegisterCount: 125 }; }
function genModelId() { return 'model_' + Date.now() + '_' + Math.floor(Math.random() * 1000); }
function uuid() { return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => { const r = Math.random() * 16 | 0; const v = c === 'x' ? r : (r & 0x3 | 0x8); return v.toString(16); }); }
function nextProfileId() { return uuid(); }
function nextProfileIndex() { let n = 0; while (state.models.some(m => m.profile && String(m.profile.profileIndex) === String(n))) n++; return n; }
function deepCopy(o) { return JSON.parse(JSON.stringify(o)); }
function loadModelIntoBuffer(m) {
  state.profile = deepCopy(m.profile);
  state.properties = deepCopy(m.properties);
}
function saveBufferIntoModel() {
  syncProfileFromForm();
  const m = state.models.find(x => x.id === state.editingId);
  if (!m) return;
  m.profile = deepCopy(state.profile);
  m.properties = deepCopy(state.properties);
}
function newModel() {
  const prof = emptyProfile();
  prof.profileId = nextProfileId();
  prof.profileIndex = nextProfileIndex();
  const m = {
    id: genModelId(),
    profile: prof,
    properties: [
      { id:'online', name:'在线状态', description:'0-离线 1-在线', dataType:'bool', unit:'', accessMode:'r', dataLength:'', base:'0', coefficient:'1', readFunctionCode:'', writeFunctionCode:'', registerBase:'', registerOffset:'', byteOrder:'' }
    ]
  };
  state.models.push(m);
  state.editingId = m.id;
  loadModelIntoBuffer(m);
  enterWizard();
}
function editModel(id) {
  const m = state.models.find(x => x.id === id);
  if (!m) return;
  state.editingId = id;
  loadModelIntoBuffer(m);
  enterWizard();
}
function deleteModel(id) {
  if (!confirm('确定删除该设备模型？此操作不可恢复。')) return;
  const m = state.models.find(x => x.id === id);
  state.models = state.models.filter(m => m.id !== id);
  if (state.editingId === id) state.editingId = null;
  if (m && m.profile && m.profile.profileId) apiDelete('/models/' + encodeURIComponent(m.profile.profileId)).catch(() => {});
  renderModelList();
}
function enterWizard() {
  document.getElementById('device-landing').classList.add('d-none');
  document.getElementById('device-wizard').classList.remove('d-none');
  document.getElementById('wizard-title').textContent = state.profile.name || '新建设备模型';
  currentStep = 1;
  fillProfileForm();
  renderProps();
  renderStep();
}
function showLanding() {
  document.getElementById('device-wizard').classList.add('d-none');
  document.getElementById('device-landing').classList.remove('d-none');
}
function backToList() {
  saveBufferIntoModel();
  const m = state.models.find(x => x.id === state.editingId);
  // discard a freshly-created model left completely empty
  if (m && !m.profile.name && !m.properties.length) {
    state.models = state.models.filter(x => x.id !== state.editingId);
  }
  state.editingId = null;
  showLanding();
  renderModelList();
}
function renderModelList() {
  const wrap = document.getElementById('model-cards');
  document.getElementById('model-count').textContent = state.models.length;
  document.getElementById('model-empty').classList.toggle('d-none', state.models.length > 0);
  wrap.innerHTML = state.models.map(m => {
    const p = m.profile || {};
    const tags = [
      `<span class="mc-tag iface">${escapeHtml(ifaceLabel(p.interfaceType))}</span>`,
      p.protocolType ? `<span class="mc-tag">${escapeHtml(p.protocolType)}</span>` : '',
      p.protocolVersion ? `<span class="mc-tag">${escapeHtml(p.protocolVersion)}</span>` : ''
    ].join('');
    const desc = p.description ? `<div class="model-card-desc">${escapeHtml(p.description)}</div>` : '';
    return `
      <div class="col-md-6 col-xl-4">
        <div class="model-card">
          <div class="model-card-top">
            <div class="model-card-icon"><i class="bi bi-cpu"></i></div>
            <div class="min-w-0">
              <div class="model-card-title">${escapeHtml(p.name || '未命名设备模型')}</div>
              <div class="model-card-sub">${escapeHtml(p.manufacturer || '未填写厂商')}</div>
            </div>
          </div>
          <div class="model-card-tags">${tags}</div>
          ${desc}
          <div class="model-card-stats">
            <div class="mc-stat"><div class="num">${m.properties.length}</div><div class="lbl">属性</div></div>
          </div>
          <div class="model-card-actions">
            <button class="btn btn-primary btn-sm flex-grow-1" onclick="editModel('${m.id}')"><i class="bi bi-gear me-1"></i>配置</button>
            <button class="btn btn-outline-danger btn-sm" onclick="deleteModel('${m.id}')" title="删除"><i class="bi bi-trash"></i></button>
          </div>
        </div>
      </div>`;
  }).join('');
}

function importProject(input) {
  const file = input.files[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = e => {
    try {
      const data = JSON.parse(e.target.result);
      if ((state.properties.length) && !confirm('导入将覆盖当前配置，是否继续？')) { input.value = ''; return; }
      state.profile = Object.assign(emptyProfile(), data.profile || {});
      state.properties = Array.isArray(data.properties) ? data.properties : [];
      fillProfileForm();
      renderProps();
      alert('配置导入成功');
    } catch (err) {
      alert('导入失败：JSON 解析错误 - ' + err.message);
    }
    input.value = '';
  };
  reader.readAsText(file);
}

/* ══════════════ Step 2 · Properties ══════════════ */
function openPropModal(idx = -1) {
  propEditIndex = idx;
  const form = document.getElementById('form-prop');
  form.classList.remove('was-validated');
  const p = idx >= 0 ? state.properties[idx]
    : { id:'', name:'', description:'', dataType:'', unit:'', accessMode:'', dataLength:'', base:'0', coefficient:'1', readFunctionCode:'', writeFunctionCode:'', registerBase:'', registerOffset:'', byteOrder:'' };
  setVal('pm-index', idx >= 0 ? idx : state.properties.length);
  setVal('pm-id', p.id); setVal('pm-name', p.name); setVal('pm-desc', p.description);
  setVal('pm-dataType', p.dataType); setVal('pm-unit', p.unit); setVal('pm-access', p.accessMode);
  setVal('pm-length', p.dataLength);
  setVal('pm-base', p.base); setVal('pm-coef', p.coefficient);
  setVal('pm-readfunc', p.readFunctionCode); setVal('pm-writefunc', p.writeFunctionCode);
  setVal('pm-regbase', p.registerBase); setVal('pm-regoffset', p.registerOffset); setVal('pm-byteorder', p.byteOrder);
  document.getElementById('pm-id').readOnly = isLocked(idx);
  document.getElementById('propModalTitle').textContent = idx >= 0 ? '编辑属性' : '添加属性';
  propModal.show();
}
function saveProp() {
  const form = document.getElementById('form-prop');
  form.classList.add('was-validated');
  if (!form.checkValidity()) return;
  const id = val('pm-id');
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(id)) { alert('属性ID 格式非法，需以字母或下划线开头'); return; }
  const dup = state.properties.findIndex(p => p.id === id);
  if (dup >= 0 && dup !== propEditIndex) { alert('属性ID 已存在：' + id); return; }
  const p = {
    id, name: val('pm-name'), description: val('pm-desc'), dataType: val('pm-dataType'), unit: val('pm-unit'),
    accessMode: val('pm-access'), dataLength: val('pm-length'), base: val('pm-base') || '0', coefficient: val('pm-coef') || '1',
    readFunctionCode: val('pm-readfunc'), writeFunctionCode: val('pm-writefunc'), registerBase: parseRegAddr(val('pm-regbase')), registerOffset: parseRegAddr(val('pm-regoffset')), byteOrder: val('pm-byteorder')
  };
  if (propEditIndex >= 0) state.properties[propEditIndex] = p; else state.properties.push(p);
  propModal.hide();
  renderProps();
}
function deleteProp(idx) {
  if (isLocked(idx)) { alert('默认属性不可删除'); return; }
  if (!confirm('确定删除该属性？')) return;
  state.properties.splice(idx, 1);
  renderProps();
}
function renderProps() {
  const tb = document.getElementById('props-tbody');
  const empty = document.getElementById('props-empty');
  const wrap = document.getElementById('props-table-wrap');
  if (!state.properties.length) { tb.innerHTML = ''; empty.classList.remove('d-none'); wrap.classList.add('d-none'); return; }
  empty.classList.add('d-none'); wrap.classList.remove('d-none');
  tb.innerHTML = state.properties.map((p, i) => {
    return `<tr>
      <td>${i}</td>
      <td><code>${escapeHtml(p.id)}</code></td>
      <td>${escapeHtml(p.name)}</td>
      <td>${escapeHtml(dataTypeLabel(p.dataType))}</td>
      <td>${escapeHtml(p.unit || '—')}</td>
      <td><span class="badge badge-${escapeHtml(p.accessMode)}">${escapeHtml((p.accessMode || '').toUpperCase())}</span></td>
      <td>${escapeHtml(String(p.coefficient ?? '1'))}</td>
      <td class="text-nowrap">
        ${isLocked(i) ? '<span class="text-muted" title="默认属性不可删除"><i class="bi bi-lock"></i></span>' : `<button class="btn btn-sm btn-link p-0 me-2" onclick="openPropModal(${i})" title="编辑"><i class="bi bi-pencil"></i></button>
        <button class="btn btn-sm btn-link p-0 text-danger" onclick="deleteProp(${i})" title="删除"><i class="bi bi-trash"></i></button>`}
      </td>
    </tr>`;
  }).join('');
}

/* ─── CSV ─── */
function csvCell(v) { const s = (v === null || v === undefined) ? '' : String(v); return '"' + s.replace(/"/g, '""') + '"'; }
function exportPropsCsv() {
  if (!state.properties.length) { alert('暂无属性可导出'); return; }
  const lines = [CSV_HEADERS.map(csvCell).join(',')];
  state.properties.forEach(p => {
    lines.push([p.id, p.name, p.description, DT_LABEL[p.dataType] || p.dataType, p.dataLength, ACCESS_LABEL[p.accessMode] || p.accessMode,
      p.base, p.coefficient, p.unit, p.readFunctionCode, p.writeFunctionCode, p.registerBase, p.registerOffset, p.byteOrder
    ].map(csvCell).join(','));
  });
  downloadBlob(new Blob(['\uFEFF' + lines.join('\r\n')], { type: 'text/csv;charset=utf-8' }), sanitize(state.profile.name) + '-properties.csv');
}
function parseCsv(text) {
  const rows = []; let row = []; let field = ''; let i = 0; let inQuotes = false;
  text = text.replace(/^\uFEFF/, '');
  while (i < text.length) {
    const c = text[i];
    if (inQuotes) {
      if (c === '"') { if (text[i + 1] === '"') { field += '"'; i += 2; continue; } inQuotes = false; i++; continue; }
      field += c; i++; continue;
    }
    if (c === '"') { inQuotes = true; i++; continue; }
    if (c === ',') { row.push(field); field = ''; i++; continue; }
    if (c === '\r') { i++; continue; }
    if (c === '\n') { row.push(field); rows.push(row); row = []; field = ''; i++; continue; }
    field += c; i++;
  }
  if (field.length > 0 || row.length > 0) { row.push(field); rows.push(row); }
  return rows;
}
function importPropsCsv(input) {
  const file = input.files[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = e => {
    try {
      const rows = parseCsv(e.target.result).filter(r => r.some(c => c.trim() !== ''));
      if (rows.length < 2) throw new Error('CSV 内容为空或缺少数据行');
      const headers = rows[0].map(h => h.trim());
      const fieldIdx = {};
      headers.forEach((h, i) => { const f = CSV_FIELD_MAP[h] || CSV_FIELD_MAP[h.toLowerCase()]; if (f) fieldIdx[f] = i; });
      ['id', 'name', 'dataType', 'accessMode'].forEach(req => { if (!(req in fieldIdx)) throw new Error('缺少必需列：' + req); });
      const imported = [];
      const seen = new Set();
      for (let r = 1; r < rows.length; r++) {
        const row = rows[r];
        const get = f => (f in fieldIdx) ? (row[fieldIdx[f]] ?? '').trim() : '';
        const id = get('id');
        if (!id) throw new Error(`第 ${r + 1} 行：属性ID 不能为空`);
        if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(id)) throw new Error(`第 ${r + 1} 行：属性ID 格式非法（${id}）`);
        if (seen.has(id)) throw new Error('CSV 中存在重复属性ID：' + id);
        seen.add(id);
        const dtRaw = get('dataType');
        const dt = DT_LABEL[dtRaw] ? dtRaw : (DT_FROM_LABEL[dtRaw] || dtRaw);
        if (!DT_LABEL[dt]) throw new Error(`第 ${r + 1} 行：数据类型非法（${dtRaw}）`);
        const accRaw = get('accessMode');
        const acc = ACCESS_FROM_LABEL[accRaw] || accRaw;
        if (!ACCESS_LABEL[acc]) throw new Error(`第 ${r + 1} 行：读写属性非法（${accRaw}）`);
        imported.push({
          id, name: get('name'), description: get('description'), dataType: dt, unit: get('unit'), accessMode: acc,
          base: get('base') || '0', coefficient: get('coefficient') || '1',
          registerAddress: get('registerAddress'),
          dataSize: get('dataSize'), byteOrder: get('byteOrder'), bitOffset: get('bitOffset')
        });
      }
      if (state.properties.length && !confirm(`将导入 ${imported.length} 条属性并覆盖当前 ${state.properties.length} 条，是否继续？`)) { input.value = ''; return; }
      state.properties = imported;
      renderProps();
      alert(`成功导入 ${imported.length} 条属性`);
    } catch (err) {
      alert('CSV 导入失败：' + err.message);
    }
    input.value = '';
  };
  reader.readAsText(file);
}

/* ══════════════ Shared helpers ══════════════ */
function isLocked(idx) { return idx === 0; }

/* ══════════════ Step 3 · Preview & Export ══════════════ */
function buildCollectorConfig() {
  syncProfileFromForm();
  return {
    profile: {
      profileIndex: state.profile.profileIndex === '' ? null : toNum(state.profile.profileIndex, null),
      profileId: state.profile.profileId || '',
      name: state.profile.name,
      manufacturer: state.profile.manufacturer || '',
      description: state.profile.description || '',
      deviceType: state.profile.deviceType || '',
      deviceModel: state.profile.deviceModel || '',
      ratedPower: toNum(state.profile.ratedPower, null),
      interfaceType: state.profile.interfaceType,
      protocolType: state.profile.protocolType,
      protocolVersion: state.profile.protocolVersion || '',
      maxRegisterCount: toNum(state.profile.maxRegisterCount, 125)
    },
    properties: state.properties.map((p, i) => ({
      index: i,
      id: p.id,
      name: p.name,
      description: p.description || '',
      dataType: p.dataType,
      unit: p.unit || '',
      accessMode: p.accessMode,
      dataLength: (p.dataLength === '' || p.dataLength === undefined) ? null : p.dataLength,
      base: toNum(p.base, 0),
      coefficient: toNum(p.coefficient, 1),
      readFunctionCode: p.readFunctionCode || '',
      writeFunctionCode: p.writeFunctionCode || '',
      registerBase: parseRegAddr(p.registerBase) || '',
      registerOffset: (p.registerOffset === '' || p.registerOffset === undefined) ? null : toNum(parseRegAddr(p.registerOffset), null),
      byteOrder: p.byteOrder || ''
    }))
  };
}
function renderPreview() {
  document.getElementById('pre-collector').textContent = JSON.stringify(buildCollectorConfig(), null, 2);
  document.getElementById('sum-name').textContent = state.profile.name || '—';
  document.getElementById('sum-props').textContent = state.properties.length;
}
function saveDeviceModel() {
  if (!validateProfile()) { goStep(1); return; }
  syncProfileFromForm();
  saveBufferIntoModel();
  const m = state.models.find(x => x.id === state.editingId);
  if (!m) { alert('未找到当前设备模型'); return; }
  apiPost('/models', modelToPayload(m))
    .then(() => { alert('设备模型已保存到数据库'); backToList(); })
    .catch(e => alert('保存失败：' + e.message));
}
function exportDeviceModel() {
  if (!validateProfile()) { goStep(1); return; }
  downloadJson(buildCollectorConfig(), sanitize(state.profile.name) + '.json');
}
