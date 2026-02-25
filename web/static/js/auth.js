
function getCsrf() {
  return (document.cookie.split('; ').find(s => s.startsWith('csrf=')) || '').split('=')[1] || '';
}

// Custom event for login state changes
const loginStateChanged = new Event('loginStateChanged');

async function whoami() {
  try {
    const me = await j('/api/me');

    if (me.authenticated) {
      isAdminUser = true;
      $('#welcome').textContent = 'Welcome, ' + me.user;
      $('#loginBtn').classList.add('hidden');
      $('#logoutBtn').classList.remove('hidden');
      applyAdminUIState();
      document.dispatchEvent(loginStateChanged);
      loadAlertsConfig();
      loadResourcesConfig();
    } else {
      isAdminUser = false;
      $('#welcome').textContent = 'Public view';
      $('#loginBtn').classList.remove('hidden');
      $('#logoutBtn').classList.add('hidden');
      applyAdminUIState();

      // Reset login form
      const dlg = document.getElementById('loginModal');
      if (dlg) {
        const submitBtn = $('#doLogin', dlg);
        if (submitBtn) {
          submitBtn.disabled = false;
          submitBtn.textContent = 'Sign In';
        }
        const errorEl = $('#loginError', dlg);
        if (errorEl) {
          errorEl.textContent = '';
          errorEl.classList.add('hidden');
        }
        $('#u', dlg).value = '';
        $('#p', dlg).value = '';
      }
    }
  } catch (e) {
    console.error('Failed to fetch user info:', e.message);
  }
}

async function handleButtonAction(btn, action, successMsg) {
  btn.disabled = true;
  btn.classList.add('loading');
  try {
    await action();
    showToast(successMsg);
  } catch (err) {
    console.error(err);
    let msg = err?.message || 'Action failed';
    if (err?.body) {
      if (typeof err.body === 'string') {
        msg = err.body;
      } else if (typeof err.body === 'object') {
        msg = err.body.message || err.body.error || msg;
      }
    }
    showToast(msg, 'error');
  } finally {
    btn.disabled = false;
    btn.classList.remove('loading');
  }
}

async function ingestAll() {
  const btn = $('#ingestNowTab') || $('#ingestNow');
  await handleButtonAction(
    btn,
    async () => {
      await j('/api/admin/ingest-now', {
        method: 'POST',
        headers: { 'X-CSRF-Token': getCsrf() }
      });
      await refresh();
    },
    'Ingestion completed successfully'
  );
}

async function resetRecent() {
  const btn = $('#resetRecentTab') || $('#resetRecent');
  await handleButtonAction(
    btn,
    async () => {
      await j('/api/admin/reset-recent', {
        method: 'POST',
        headers: { 'X-CSRF-Token': getCsrf() }
      });
      await refresh();
    },
    'Recent incidents reset successfully'
  );
}

/* Security Tab Functions */
async function loadSecurityData() {
  await Promise.all([loadBlocks(), loadWhitelist(), loadBlacklist()]);
}

async function loadBlocks() {
  const container = $('#blocksList');
  if (!container) return;

  try {
    const data = await j('/api/admin/blocks');
    const blocks = data.blocks || [];

    if (blocks.length === 0) {
      container.innerHTML = '<div class="muted">No temporary blocks</div>';
      return;
    }

    container.innerHTML = blocks.map(block => `
      <div class="block-item">
        <div class="block-info">
          <strong>${escapeHtml(block.ip)}</strong>
          <span class="muted">Attempts: ${block.attempts} â€¢ Expires: ${new Date(block.expires_at).toLocaleString()}</span>
        </div>
        <button class="btn danger small" data-action="unblock" data-ip="${escapeHtml(block.ip)}">Unblock</button>
      </div>
    `).join('');
  } catch (err) {
    container.innerHTML = '<div class="muted">Failed to load blocks</div>';
  }
}

async function unblockIP(ip) {
  try {
    await j('/api/admin/unblock', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip })
    });
    showToast('IP unblocked');
    loadBlocks();
  } catch (err) {
    showToast('Failed to unblock IP', 'error');
  }
}

async function clearAllBlocks() {
  if (!confirm('Are you sure you want to clear all temporary blocks?')) return;
  try {
    await j('/api/admin/clear-blocks', {
      method: 'POST',
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    showToast('All blocks cleared');
    loadBlocks();
  } catch (err) {
    showToast('Failed to clear blocks', 'error');
  }
}

async function loadWhitelist() {
  const container = $('#whitelistList');
  if (!container) return;

  try {
    const data = await j('/api/admin/whitelist');
    const list = data.whitelist || [];

    if (list.length === 0) {
      container.innerHTML = '<div class="muted">No whitelisted IPs</div>';
      return;
    }

    container.innerHTML = list.map(item => `
      <div class="block-item">
        <div class="block-info">
          <strong>${escapeHtml(item.ip)}</strong>
          <span class="muted">${item.note ? escapeHtml(item.note) : 'No note'} â€¢ Added: ${new Date(item.created_at).toLocaleDateString()}</span>
        </div>
        <button class="btn danger small" data-action="remove-whitelist" data-ip="${escapeHtml(item.ip)}">Remove</button>
      </div>
    `).join('');
  } catch (err) {
    container.innerHTML = '<div class="muted">Failed to load whitelist</div>';
  }
}

async function addToWhitelist() {
  const ipInput = $('#whitelistIP');
  const noteInput = $('#whitelistNote');
  const ip = ipInput.value.trim();
  const note = noteInput.value.trim();

  if (!ip) {
    showToast('Please enter an IP address', 'error');
    return;
  }

  try {
    await j('/api/admin/whitelist', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip, note })
    });
    ipInput.value = '';
    noteInput.value = '';
    showToast('IP added to whitelist');
    loadWhitelist();
  } catch (err) {
    showToast('Failed to add to whitelist', 'error');
  }
}

async function removeFromWhitelist(ip) {
  try {
    await j('/api/admin/whitelist', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip })
    });
    showToast('IP removed from whitelist');
    loadWhitelist();
  } catch (err) {
    showToast('Failed to remove from whitelist', 'error');
  }
}

async function loadBlacklist() {
  const container = $('#blacklistList');
  if (!container) return;

  try {
    const data = await j('/api/admin/blacklist');
    const list = data.blacklist || [];

    if (list.length === 0) {
      container.innerHTML = '<div class="muted">No blacklisted IPs</div>';
      return;
    }

    container.innerHTML = list.map(item => `
      <div class="block-item">
        <div class="block-info">
          <strong>${escapeHtml(item.ip)}${item.permanent ? '<span class="badge">PERMANENT</span>' : ''}</strong>
          <span class="muted">${item.note ? escapeHtml(item.note) : 'No note'} â€¢ Added: ${new Date(item.created_at).toLocaleDateString()}</span>
        </div>
        <button class="btn danger small" data-action="remove-blacklist" data-ip="${escapeHtml(item.ip)}">Remove</button>
      </div>
    `).join('');
  } catch (err) {
    container.innerHTML = '<div class="muted">Failed to load blacklist</div>';
  }
}

async function addToBlacklist() {
  const ipInput = $('#blacklistIP');
  const noteInput = $('#blacklistNote');
  const permanentInput = $('#blacklistPermanent');
  const ip = ipInput.value.trim();
  const note = noteInput.value.trim();
  const permanent = permanentInput.checked;

  if (!ip) {
    showToast('Please enter an IP address', 'error');
    return;
  }

  try {
    await j('/api/admin/blacklist', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip, note, permanent })
    });
    ipInput.value = '';
    noteInput.value = '';
    permanentInput.checked = false;
    showToast('IP added to blacklist');
    loadBlacklist();
  } catch (err) {
    showToast('Failed to add to blacklist', 'error');
  }
}

async function removeFromBlacklist(ip) {
  try {
    await j('/api/admin/blacklist', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': getCsrf() },
      body: JSON.stringify({ ip })
    });
    showToast('IP removed from blacklist');
    loadBlacklist();
  } catch (err) {
    showToast('Failed to remove from blacklist', 'error');
  }
}
