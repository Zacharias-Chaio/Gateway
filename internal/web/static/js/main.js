
/* ══════════════ init ══════════════ */
function doLogin() {
  if (val('login-user') === 'admin' && val('login-pass') === '666') {
    sessionStorage.setItem('gw_auth', '1');
    document.getElementById('login-overlay').classList.add('d-none');
    document.getElementById('login-err').classList.add('d-none');
  } else {
    document.getElementById('login-err').classList.remove('d-none');
  }
}
function doLogout() {
  sessionStorage.removeItem('gw_auth');
  document.getElementById('login-pass').value = '';
  document.getElementById('login-err').classList.add('d-none');
  document.getElementById('login-overlay').classList.remove('d-none');
  document.getElementById('login-user').focus();
}
function init() {
  propModal = new bootstrap.Modal(document.getElementById('propModal'));
  Promise.all([loadModels(), loadSettings(), loadSystemInfo()]).then(() => loadChannels()).then(() => {
    switchSection('device');
    showLanding();
    renderModelList();
    renderChannelList();
  });
}
document.addEventListener('DOMContentLoaded', () => {
  if (sessionStorage.getItem('gw_auth') === '1') document.getElementById('login-overlay').classList.add('d-none');
  init();
});
