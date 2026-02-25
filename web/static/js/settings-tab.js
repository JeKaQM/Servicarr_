// ============ Settings Tab Handlers ============

// Save App Name
async function saveAppName() {
  const appNameInput = $('#appNameInput');
  const statusEl = $('#appNameStatus');
  const appName = appNameInput?.value?.trim() || 'Service Status';

  try {
    const res = await fetch('/api/admin/settings/app-name', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ app_name: appName })
    });

    const data = await res.json();
    if (res.ok) {
      showStatus(statusEl, 'App name saved! Refreshing...', 'success');
      // Update the page title and header immediately
      document.title = data.app_name || appName;
      const appTitle = $('#appTitle');
      if (appTitle) appTitle.textContent = data.app_name || appName;
      // Reload to ensure all references are updated
      setTimeout(() => window.location.reload(), 1000);
    } else {
      showStatus(statusEl, data.error || 'Failed to save app name', 'error');
    }
  } catch (err) {
    showStatus(statusEl, 'Network error: ' + err.message, 'error');
  }
}

// Change Password
async function changePassword() {
  const currentPassword = $('#currentPassword')?.value;
  const newPassword = $('#newPassword')?.value;
  const confirmPassword = $('#confirmPassword')?.value;
  const statusEl = $('#passwordStatus');

  if (!currentPassword || !newPassword || !confirmPassword) {
    showStatus(statusEl, 'Please fill in all fields', 'error');
    return;
  }

  if (newPassword !== confirmPassword) {
    showStatus(statusEl, 'New passwords do not match', 'error');
    return;
  }

  if (newPassword.length < 6) {
    showStatus(statusEl, 'Password must be at least 6 characters', 'error');
    return;
  }

  try {
    const res = await fetch('/api/admin/settings/password', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword
      })
    });

    const data = await res.json();
    if (res.ok) {
      showStatus(statusEl, 'Password changed successfully!', 'success');
      $('#currentPassword').value = '';
      $('#newPassword').value = '';
      $('#confirmPassword').value = '';
    } else {
      showStatus(statusEl, data.error || 'Failed to change password', 'error');
    }
  } catch (err) {
    showStatus(statusEl, 'Network error: ' + err.message, 'error');
  }
}

// Export Database
async function exportDatabase() {
  const statusEl = $('#backupStatus');
  try {
    showStatus(statusEl, 'Preparing export...', 'info');

    const res = await fetch('/api/admin/settings/export', { credentials: 'same-origin' });
    if (!res.ok) {
      const data = await res.json();
      showStatus(statusEl, data.error || 'Export failed', 'error');
      return;
    }

    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    const timestamp = new Date().toISOString().slice(0, 10);
    a.download = `servicarr-backup-${timestamp}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);

    showStatus(statusEl, 'Database exported successfully!', 'success');
  } catch (err) {
    showStatus(statusEl, 'Export failed: ' + err.message, 'error');
  }
}

// Import Database
let selectedImportFile = null;

function handleImportFileSelect(event) {
  const file = event.target.files[0];
  if (!file) return;

  selectedImportFile = file;
  const fileNameEl = $('#importFileName');
  if (fileNameEl) fileNameEl.textContent = file.name;

  const dialog = $('#confirmImportDialog');
  if (dialog) dialog.showModal();
}

async function confirmImportDatabase() {
  const statusEl = $('#backupStatus');
  const errorEl = $('#importDbError');

  if (!selectedImportFile) {
    if (errorEl) {
      errorEl.textContent = 'No file selected';
      errorEl.classList.remove('hidden');
    }
    return;
  }

  try {
    const formData = new FormData();
    formData.append('backup', selectedImportFile);

    const res = await fetch('/api/admin/settings/import', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'X-CSRF-Token': getCsrf() },
      body: formData
    });

    const ct = res.headers.get('content-type') || '';
    if (!ct.includes('application/json')) {
      throw new Error(res.status === 413
        ? 'File too large for your reverse proxy. Increase its upload limit (e.g. client_max_body_size in Nginx or Cloudflare upload size).'
        : `Server returned ${res.status} with a non-JSON response. Check your reverse proxy configuration.`);
    }
    const data = await res.json();

    const dialog = $('#confirmImportDialog');
    if (dialog) dialog.close();

    if (res.ok) {
      showStatus(statusEl, 'Database imported successfully! Reloading...', 'success');
      // Reload page after import to reflect changes
      setTimeout(() => window.location.reload(), 1500);
    } else {
      showStatus(statusEl, data.error || 'Import failed', 'error');
    }
  } catch (err) {
    showStatus(statusEl, 'Import failed: ' + err.message, 'error');
    const dialog = $('#confirmImportDialog');
    if (dialog) dialog.close();
  }

  // Reset file input
  const fileInput = $('#importDbFile');
  if (fileInput) fileInput.value = '';
  selectedImportFile = null;
}

// Reset Database
function openResetDbDialog() {
  const dialog = $('#confirmResetDialog');
  if (dialog) {
    $('#confirmResetPassword').value = '';
    $('#resetDbError')?.classList.add('hidden');
    dialog.showModal();
  }
}

async function confirmResetDatabase() {
  const password = $('#confirmResetPassword')?.value;
  const errorEl = $('#resetDbError');
  const statusEl = $('#backupStatus');

  if (!password) {
    if (errorEl) {
      errorEl.textContent = 'Password is required';
      errorEl.classList.remove('hidden');
    }
    return;
  }

  try {
    const res = await fetch('/api/admin/settings/reset', {
      method: 'POST',
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ password })
    });

    const data = await res.json();

    const dialog = $('#confirmResetDialog');
    if (dialog) dialog.close();

    if (res.ok) {
      showStatus(statusEl, 'Database reset successfully! Redirecting to setup...', 'success');
      // Redirect to setup page after reset
      setTimeout(() => window.location.href = '/setup', 1500);
    } else {
      if (errorEl) {
        errorEl.textContent = data.error || 'Reset failed';
        errorEl.classList.remove('hidden');
      }
    }
  } catch (err) {
    if (errorEl) {
      errorEl.textContent = 'Network error: ' + err.message;
      errorEl.classList.remove('hidden');
    }
  }
}

function showStatus(el, message, type) {
  if (!el) return;
  el.textContent = message;
  el.className = 'status-message ' + type;
  el.classList.remove('hidden');

  // Auto-hide success messages after 5 seconds
  if (type === 'success') {
    setTimeout(() => el.classList.add('hidden'), 5000);
  }
}

// Initialize Settings Tab
function initSettingsTab() {
  // Save app name
  const saveAppNameBtn = $('#saveAppNameBtn');
  if (saveAppNameBtn) {
    saveAppNameBtn.addEventListener('click', saveAppName);
  }

  // Change password
  const changePasswordBtn = $('#changePasswordBtn');
  if (changePasswordBtn) {
    changePasswordBtn.addEventListener('click', changePassword);
  }

  // Export database
  const exportBtn = $('#exportDbBtn');
  if (exportBtn) {
    exportBtn.addEventListener('click', exportDatabase);
  }

  // Import database
  const importInput = $('#importDbFile');
  if (importInput) {
    importInput.addEventListener('change', handleImportFileSelect);
  }

  const cancelImport = $('#cancelImportDb');
  if (cancelImport) {
    cancelImport.addEventListener('click', () => {
      const dialog = $('#confirmImportDialog');
      if (dialog) dialog.close();
      selectedImportFile = null;
    });
  }

  const confirmImport = $('#confirmImportDb');
  if (confirmImport) {
    confirmImport.addEventListener('click', confirmImportDatabase);
  }

  // Reset database
  const resetBtn = $('#resetDbBtn');
  if (resetBtn) {
    resetBtn.addEventListener('click', openResetDbDialog);
  }

  const cancelReset = $('#cancelResetDb');
  if (cancelReset) {
    cancelReset.addEventListener('click', () => {
      const dialog = $('#confirmResetDialog');
      if (dialog) dialog.close();
    });
  }

  const confirmReset = $('#confirmResetDb');
  if (confirmReset) {
    confirmReset.addEventListener('click', confirmResetDatabase);
  }

  // Close dialogs on backdrop click
  const resetDialog = $('#confirmResetDialog');
  if (resetDialog) {
    resetDialog.addEventListener('click', (e) => {
      if (e.target === resetDialog) resetDialog.close();
    });
  }

  const importDialog = $('#confirmImportDialog');
  if (importDialog) {
    importDialog.addEventListener('click', (e) => {
      if (e.target === importDialog) importDialog.close();
    });
  }
}

// Handle browser back/forward cache (bfcache) restoration
// When the browser restores from cache, force reload the config to ensure correct visibility
window.addEventListener('pageshow', (event) => {
  if (event.persisted) {
    console.log('[Resources] Page restored from bfcache, reloading config');
    loadResourcesConfig();
  }
});
