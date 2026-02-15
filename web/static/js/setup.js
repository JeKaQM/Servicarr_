document.addEventListener('DOMContentLoaded', function() {
    const $ = s => document.querySelector(s);
    const $$ = s => document.querySelectorAll(s);
    
    let currentStep = 1;
    let selectedServiceType = '';
    let credentials = {};
    let importedFromBackup = false;
    let pendingImportFile = null;

    // Restore button click - open file picker
    $('#restoreBtn').addEventListener('click', () => {
      $('#restoreFile').click();
    });

    // File selected - show confirmation dialog
    $('#restoreFile').addEventListener('change', (e) => {
      const file = e.target.files[0];
      if (!file) return;

      pendingImportFile = file;
      $('#importFileName').textContent = file.name;
      $('#importConfirmDialog').showModal();
    });

    // Cancel import
    $('#cancelImport').addEventListener('click', () => {
      $('#importConfirmDialog').close();
      $('#restoreFile').value = '';
      pendingImportFile = null;
    });

    // Confirm import
    $('#confirmImport').addEventListener('click', async () => {
      if (!pendingImportFile) return;

      const dialog = $('#importConfirmDialog');
      const confirmBtn = $('#confirmImport');
      confirmBtn.disabled = true;
      confirmBtn.textContent = 'Importing...';

      const statusEl = $('#restoreStatus');

      try {
        const formData = new FormData();
        formData.append('backup', pendingImportFile);

        const res = await fetch('/api/setup/import', {
          method: 'POST',
          body: formData
        });

        const data = await res.json();
        dialog.close();

        if (res.ok) {
          importedFromBackup = true;
          
          // Hide restore section, show success banner
          $('#restoreSection').classList.add('hidden');
          $('#orDivider').classList.add('hidden');
          
          const banner = $('#restoredBanner');
          banner.classList.remove('hidden');
          $('#restoredInfo').textContent = `${data.services_imported || 0} services and your settings have been imported.`;
          
          // Update step title and description
          $('#step1Title').textContent = 'Create New Admin Account';
          $('#step1Desc').textContent = 'For security, you must create new login credentials.';
          
        } else {
          statusEl.classList.remove('hidden');
          statusEl.classList.add('error');
          statusEl.textContent = data.error || 'Import failed';
        }
      } catch (err) {
        dialog.close();
        statusEl.classList.remove('hidden');
        statusEl.classList.add('error');
        statusEl.textContent = 'Import failed: ' + err.message;
      }

      // Reset
      confirmBtn.disabled = false;
      confirmBtn.textContent = 'Import Backup';
      $('#restoreFile').value = '';
      pendingImportFile = null;
    });

    // Service template data
    const templates = {
      plex: { name: 'Plex', icon: 'https://cdn.simpleicons.org/plex/E5A00D' },
      jellyfin: { name: 'Jellyfin', icon: 'https://cdn.simpleicons.org/jellyfin/00A4DC' },
      sonarr: { name: 'Sonarr', icon: 'https://cdn.simpleicons.org/sonarr/00CCFF' },
      radarr: { name: 'Radarr', icon: 'https://cdn.simpleicons.org/radarr/FFC230' },
      overseerr: { name: 'Overseerr', icon: 'https://cdn.simpleicons.org/overseerr/5B4BB5' },
      custom: { name: 'Custom Service', icon: '' }
    };

    function showStep(step) {
      $$('.step').forEach(s => s.classList.remove('active'));
      $(`#step${step}`).classList.add('active');
      
      $$('.step-dot').forEach((dot, i) => {
        dot.classList.remove('active', 'completed');
        if (i + 1 < step) dot.classList.add('completed');
        if (i + 1 === step) dot.classList.add('active');
      });
      
      currentStep = step;
    }

    function showError(stepId, message) {
      const el = $(`#${stepId}Error`);
      if (el) {
        el.textContent = message;
        el.classList.remove('hidden');
      }
    }

    function hideError(stepId) {
      const el = $(`#${stepId}Error`);
      if (el) el.classList.add('hidden');
    }

    // Step 1: Create Account
    $('#step1Next').addEventListener('click', async () => {
      hideError('step1');
      
      const username = $('#username').value.trim();
      const password = $('#password').value;
      const confirmPassword = $('#confirmPassword').value;

      if (username.length < 3) {
        showError('step1', 'Username must be at least 3 characters');
        return;
      }

      if (password.length < 8) {
        showError('step1', 'Password must be at least 8 characters');
        return;
      }

      if (password !== confirmPassword) {
        showError('step1', 'Passwords do not match');
        return;
      }

      // Store credentials for later
      credentials = { username, password };
      
      // Save credentials
      const btn = $('#step1Next');
      btn.disabled = true;
      btn.textContent = 'Setting up...';

      try {
        const resp = await fetch('/api/setup', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ username, password })
        });
        
        const data = await resp.json();
        
        if (!data.success) {
          showError('step1', data.error || 'Setup failed');
          btn.disabled = false;
          btn.textContent = 'Continue';
          return;
        }
        
        // Skip step 2 if we imported from backup (services already exist)
        if (importedFromBackup) {
          showStep(3);
        } else {
          showStep(2);
        }
      } catch (e) {
        showError('step1', 'Connection error. Please try again.');
        btn.disabled = false;
        btn.textContent = 'Continue';
      }
    });

    // Service template selection
    $$('.service-template').forEach(el => {
      el.addEventListener('click', () => {
        $$('.service-template').forEach(t => t.classList.remove('selected'));
        el.classList.add('selected');
        
        selectedServiceType = el.dataset.type;
        const template = templates[selectedServiceType];
        
        if (selectedServiceType !== 'custom') {
          $('#serviceName').value = template.name;
        } else {
          $('#serviceName').value = '';
        }
      });
    });

    // Test Connection
    $('#testConnection').addEventListener('click', async () => {
      const url = $('#serviceUrl').value.trim();
      const token = $('#serviceToken').value.trim();
      const resultEl = $('#testConnectionResult');
      
      if (!url) {
        resultEl.textContent = 'Please enter a URL first';
        resultEl.className = 'test-result error';
        resultEl.classList.remove('hidden');
        return;
      }

      const btn = $('#testConnection');
      btn.disabled = true;
      btn.textContent = 'Testing...';
      resultEl.classList.add('hidden');

      try {
        const resp = await fetch('/api/admin/services/test', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            url,
            api_token: token,
            service_type: selectedServiceType || 'custom',
            check_type: 'http',
            timeout: 5
          })
        });
        
        const data = await resp.json();
        
        if (data.success) {
          let msg = '✓ Connection successful';
          if (data.status_code) msg += ` (${data.status_code})`;
          if (data.latency_ms !== undefined) msg += ` - ${data.latency_ms}ms`;
          resultEl.textContent = msg;
          resultEl.className = 'test-result success';
        } else {
          resultEl.textContent = '✗ ' + (data.error || 'Connection failed');
          resultEl.className = 'test-result error';
        }
        resultEl.classList.remove('hidden');
      } catch (e) {
        resultEl.textContent = '✗ Connection test failed';
        resultEl.className = 'test-result error';
        resultEl.classList.remove('hidden');
      } finally {
        btn.disabled = false;
        btn.textContent = 'Test';
      }
    });

    // Add Service
    $('#addService').addEventListener('click', async () => {
      hideError('step2');
      
      const name = $('#serviceName').value.trim();
      const url = $('#serviceUrl').value.trim();
      const token = $('#serviceToken').value.trim();

      if (!name || !url) {
        showError('step2', 'Please enter a name and URL');
        return;
      }

      const btn = $('#addService');
      btn.disabled = true;
      btn.textContent = 'Adding...';

      try {
        const resp = await fetch('/api/setup/service', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name,
            url,
            service_type: selectedServiceType || 'custom',
            api_token: token,
            icon_url: templates[selectedServiceType]?.icon || ''
          })
        });
        
        const data = await resp.json();
        
        if (!data.success) {
          showError('step2', data.error || 'Failed to add service');
          btn.disabled = false;
          btn.textContent = 'Add Service';
          return;
        }
        
        showStep(3);
      } catch (e) {
        showError('step2', 'Connection error. Please try again.');
        btn.disabled = false;
        btn.textContent = 'Add Service';
      }
    });

    // Skip adding service
    $('#skipService').addEventListener('click', () => {
      showStep(3);
    });

    // Go to Dashboard
    $('#goToDashboard').addEventListener('click', () => {
      window.location.href = '/';
    });

    // Enter key support
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        if (currentStep === 1) {
          $('#step1Next').click();
        }
      }
    });
    }); // End DOMContentLoaded
