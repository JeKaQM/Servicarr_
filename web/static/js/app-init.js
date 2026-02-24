document.addEventListener('DOMContentLoaded', () => {
  initLogsTab();
  initNotificationSelector();

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