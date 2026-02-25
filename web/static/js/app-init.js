// ---- Core dashboard initialization (public bundle) ----
// This must run for ALL users (public + admin) to populate cards, uptime bars, and start polling.
window.addEventListener('load', async () => {
  // Load resources config and services in parallel for fastest possible render.
  const [, services] = await Promise.all([
    loadResourcesConfig().catch(e => {
      console.warn('Resources config load failed, continuing:', e);
    }),
    loadServices().catch(e => {
      console.error('Failed to load services on init', e);
      return [];
    })
  ]);

  if (services && services.length > 0) {
    renderDynamicUptimeBars(services);
  }

  // Initialize view toggle (Cards / Hive)
  initViewToggle();

  // Now start refresh — cards are guaranteed to be in the DOM
  refresh();
  whoami();
  setInterval(refresh, REFRESH_MS);

  // Handle both click and touch events for login button
  const loginBtn = $('#loginBtn');
  if (loginBtn) {
    loginBtn.addEventListener('click', doLoginFlow);
  }

  // Handle login form submission (prevents iOS form submit)
  const loginForm = document.querySelector('#loginModal .login-form');
  if (loginForm) {
    loginForm.addEventListener('submit', (e) => {
      e.preventDefault();
      e.stopPropagation();
      submitLogin();
      return false;
    });
  }

  // Handle doLogin button
  const doLoginBtn = $('#doLogin');
  if (doLoginBtn) {
    doLoginBtn.addEventListener('click', (e) => {
      e.preventDefault();
      e.stopPropagation();
      submitLogin();
    });
  }

  // Handle cancel button
  const cancelBtn = $('#cancelLogin');
  if (cancelBtn) {
    cancelBtn.addEventListener('click', (e) => {
      e.preventDefault();
      $('#loginModal').close();
    });
  }

  // Handle both click and touch for logout
  const logoutBtn = $('#logoutBtn');
  if (logoutBtn) {
    logoutBtn.addEventListener('click', logout);
    logoutBtn.addEventListener('touchstart', (e) => {
      e.preventDefault();
      logout();
    }, { passive: false });
  }

  // Uptime filter dropdown
  const uptimeFilter = $('#uptimeFilter');
  if (uptimeFilter) {
    uptimeFilter.addEventListener('change', async (e) => {
      DAYS = parseInt(e.target.value);

      try {
        const metrics = await j(`/api/metrics?days=${DAYS}`);
        $('#window').textContent = `Last ${DAYS} days`;
        renderUptimeBars(metrics, DAYS);
      } catch (err) {
        console.error('Failed to fetch metrics for new time range', err);
        renderUptimeBars(null, DAYS);
      }
    });
  }

  // Load public banners
  loadBanners();
});

document.addEventListener('DOMContentLoaded', () => {
  // Admin-only modules may not be loaded for public visitors
  if (typeof initLogsTab === 'function') initLogsTab();
  if (typeof initNotificationSelector === 'function') initNotificationSelector();

  // Delegated click handler for dynamically-created buttons (CSP-compliant, no inline onclick)
  document.addEventListener('click', (e) => {
    const actionEl = e.target.closest('[data-action]');
    if (!actionEl) return;
    const action = actionEl.dataset.action;

    switch (action) {
      case 'unblock':
        unblockIP(actionEl.dataset.ip);
        break;
      case 'remove-whitelist':
        removeFromWhitelist(actionEl.dataset.ip);
        break;
      case 'remove-blacklist':
        removeFromBlacklist(actionEl.dataset.ip);
        break;
      case 'show-log':
        showLogDetails(actionEl);
        break;
      case 'close-modal':
        // handled by direct listener on modal creation
        break;
    }
  });
});