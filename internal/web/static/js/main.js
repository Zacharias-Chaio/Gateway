
/* ══════════════ init ══════════════ */
function doLogin() {
  if (val('login-user') === 'admin' && val('login-pass') === '666666') {
    sessionStorage.setItem('gw_auth', '1');
    document.getElementById('login-overlay').remove();
  } else {
    document.getElementById('login-err').classList.remove('d-none');
  }
}
function init() {
  propModal = new bootstrap.Modal(document.getElementById('propModal'));
  Promise.all([loadModels(), loadHardware()]).then(() => loadChannels()).then(() => {
    switchSection('device');
    showLanding();
    renderModelList();
    renderChannelList();
  });
}
document.addEventListener('DOMContentLoaded', () => {
  if (sessionStorage.getItem('gw_auth') === '1') document.getElementById('login-overlay').remove();
  init();
});
