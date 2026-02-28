function renderAdminServicesList(services) {
  const list = $('#servicesList');
  if (!list) return;

  list.innerHTML = '';
  const totalServices = services.length;

  services.forEach((svc, index) => {
    const item = document.createElement('div');
    item.className = 'service-item';
    item.dataset.id = svc.id;
    item.dataset.index = index;
    item.draggable = true;

    // Use icon HTML (with img for known types or custom icon URL)
    const iconHtml = getServiceIconHtml(svc);

    // Mask the URL for display (only show domain)
    const urlDisplay = escapeHtml(maskUrl(svc.url));
    const svcName = escapeHtml(svc.name || '');

    item.innerHTML = `
      <span class="drag-handle desktop-only">⋮⋮</span>
      <div class="reorder-buttons mobile-only">
        <button class="reorder-btn move-up" ${index === 0 ? 'disabled' : ''} title="Move up">▲</button>
        <button class="reorder-btn move-down" ${index === totalServices - 1 ? 'disabled' : ''} title="Move down">▼</button>
      </div>
      <span class="service-icon-wrap">${iconHtml}</span>
      <div class="service-info">
        <div class="service-name">${svcName}</div>
        <div class="service-url">${urlDisplay}</div>
      </div>
      <div class="service-actions">
        <button class="action-btn visibility-btn ${svc.visible ? 'visible' : 'hidden-svc'}" title="${svc.visible ? 'Hide from dashboard' : 'Show on dashboard'}">
          ${svc.visible ? '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>' : '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>'}
        </button>
        <button class="action-btn edit-btn" title="Edit service">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
        </button>
      </div>
    `;

    // Drag and drop events (desktop)
    item.addEventListener('dragstart', handleDragStart);
    item.addEventListener('dragend', handleDragEnd);
    item.addEventListener('dragover', handleDragOver);
    item.addEventListener('drop', handleDrop);
    item.addEventListener('dragenter', handleDragEnter);
    item.addEventListener('dragleave', handleDragLeave);

    // Reorder button events (mobile)
    item.querySelector('.move-up')?.addEventListener('click', () => moveService(svc.id, 'up'));
    item.querySelector('.move-down')?.addEventListener('click', () => moveService(svc.id, 'down'));

    // Visibility toggle
    item.querySelector('.visibility-btn').addEventListener('click', () => toggleServiceVisibility(svc.id, !svc.visible));

    // Edit button
    item.querySelector('.edit-btn').addEventListener('click', () => openServiceModal(svc));

    list.appendChild(item);
  });
}

// Move service up or down
async function moveService(id, direction) {
  const list = $('#servicesList');
  const items = [...list.querySelectorAll('.service-item')];
  const currentIndex = items.findIndex(item => item.dataset.id == id);

  if (currentIndex === -1) return;

  const newIndex = direction === 'up' ? currentIndex - 1 : currentIndex + 1;
  if (newIndex < 0 || newIndex >= items.length) return;

  // Get all service IDs in new order
  const newOrder = items.map(item => parseInt(item.dataset.id));
  [newOrder[currentIndex], newOrder[newIndex]] = [newOrder[newIndex], newOrder[currentIndex]];

  try {
    const orders = {};
    newOrder.forEach((serviceID, index) => {
      orders[serviceID] = index;
    });

    await j('/api/admin/services/reorder', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ orders })
    });

    await loadAllServices();
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
    });
  } catch (err) {
    console.error('Failed to reorder:', err);
    showToast('Failed to reorder services', 'error');
  }
}

// Drag and drop state
let draggedItem = null;

function handleDragStart(e) {
  draggedItem = this;
  this.classList.add('dragging');
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/html', this.innerHTML);
}

function handleDragEnd(e) {
  this.classList.remove('dragging');
  $$('.service-item').forEach(item => item.classList.remove('drag-over'));
  draggedItem = null;
}

function handleDragOver(e) {
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  return false;
}

function handleDragEnter(e) {
  this.classList.add('drag-over');
}

function handleDragLeave(e) {
  this.classList.remove('drag-over');
}

function handleDrop(e) {
  e.stopPropagation();
  e.preventDefault();

  if (draggedItem !== this) {
    const list = $('#servicesList');
    const items = [...list.querySelectorAll('.service-item')];
    const draggedIndex = items.indexOf(draggedItem);
    const targetIndex = items.indexOf(this);

    if (draggedIndex < targetIndex) {
      this.parentNode.insertBefore(draggedItem, this.nextSibling);
    } else {
      this.parentNode.insertBefore(draggedItem, this);
    }

    // Save the new order
    saveServiceOrder();
  }

  this.classList.remove('drag-over');
  return false;
}

async function saveServiceOrder() {
  const list = $('#servicesList');
  const items = [...list.querySelectorAll('.service-item')];

  const orders = {};
  items.forEach((item, index) => {
    orders[parseInt(item.dataset.id)] = index;
  });

  try {
    await j('/api/admin/services/reorder', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ orders })
    });
    showToast('Order saved');
    // Reload to reflect new order everywhere
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
    });
  } catch (e) {
    console.error('Failed to save order', e);
    showToast('Failed to save order', 'error');
  }
}

// Mask URL for display - show only host, hide path and port
function maskUrl(url) {
  try {
    const parsed = new URL(url);
    return `${parsed.protocol}//${parsed.hostname}`;
  } catch {
    return '***';
  }
}

async function toggleServiceVisibility(id, visible) {
  try {
    await j(`/api/admin/services/${id}/visibility`, {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify({ visible })
    });
    showToast(`Service ${visible ? 'shown' : 'hidden'}`);
    loadAllServices();
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
      refresh();
    });
  } catch (e) {
    console.error('Failed to toggle visibility', e);
    showToast('Failed to toggle visibility', 'error');
  }
}

// Update icon preview in the service modal
function updateIconPreview(iconUrl) {
  const preview = $('#iconPreview');
  if (!preview) return;

  if (iconUrl) {
    const safeUrl = /^(https?:\/\/|data:image\/|\/static\/)/.test(iconUrl) ? escapeHtml(iconUrl) : '';
    if (safeUrl) {
      preview.innerHTML = `<img src="${safeUrl}" class="icon-preview-img" alt="Icon preview" /><span class="icon-preview-fallback" style="display:none;">⚠️</span>`;
      preview.classList.remove('hidden');
    } else {
      preview.innerHTML = '<span class="icon-preview-fallback">Invalid URL</span>';
      preview.classList.remove('hidden');
    }
  } else {
    preview.innerHTML = '';
    preview.classList.add('hidden');
  }
}

function openServiceModal(service = null) {
  const modal = $('#serviceModal');
  if (!modal) return;

  editingServiceId = service?.id || null;

  // Update modal title
  const title = $('#serviceModalTitle');
  if (title) {
    title.textContent = service ? 'Edit Service' : 'Add Service';
  }

  // Show/hide delete button
  const deleteBtn = $('#deleteService');
  if (deleteBtn) {
    deleteBtn.classList.toggle('hidden', !service);
  }

  // Clear any previous error
  const errEl = $('#serviceError');
  if (errEl) {
    errEl.textContent = '';
    errEl.classList.add('hidden');
  }

  // Clear any previous test result
  const testResultEl = $('#testConnectionResult');
  if (testResultEl) {
    testResultEl.textContent = '';
    testResultEl.classList.add('hidden');
  }

  // Populate form
  $('#serviceTemplate').value = service?.service_type || '';
  $('#serviceName').value = service?.name || '';
  $('#serviceUrl').value = service?.url || '';

  // API token: show placeholder for existing masked tokens, never put mask in the field
  const tokenInput = $('#serviceToken');
  if (service?.api_token && service.api_token.includes('\u2022')) {
    tokenInput.value = '';
    tokenInput.placeholder = 'Token saved \u2014 leave blank to keep, or enter new token';
  } else {
    tokenInput.value = service?.api_token || '';
    tokenInput.placeholder = 'API token (if required)';
  }

  $('#serviceIconUrl').value = service?.icon_url || '';
  $('#serviceCheckType').value = service?.check_type || 'http';
  $('#serviceTimeout').value = service?.timeout || 5;
  $('#serviceInterval').value = service?.check_interval || 60;
  $('#serviceExpectedMin').value = service?.expected_min || 200;
  $('#serviceExpectedMax').value = service?.expected_max || 399;
  $('#serviceVisible').checked = service?.visible !== false;
  $('#serviceId').value = service?.id || '';
  $('#serviceType').value = service?.service_type || '';

  // Update icon preview
  updateIconPreview(service?.icon_url);

  // If editing, disable template selection
  $('#serviceTemplate').disabled = !!service;

  // Populate depends-on checkbox list
  populateDependsOnDropdown(service?.key);
  // Set selected dependencies if editing
  if (service?.depends_on) {
    const deps = service.depends_on.split(',').map(d => d.trim()).filter(Boolean);
    const container = $('#serviceDependsOnList');
    if (container) {
      container.querySelectorAll('.depends-on-cb').forEach(cb => {
        cb.checked = deps.includes(cb.value);
      });
    }
  }

  // Populate connected-to checkbox list
  populateConnectedToList(service?.key);
  // Set selected connections if editing
  if (service?.connected_to) {
    const conns = service.connected_to.split(',').map(c => c.trim()).filter(Boolean);
    const container = $('#serviceConnectedToList');
    if (container) {
      container.querySelectorAll('.connected-to-cb').forEach(cb => {
        cb.checked = conns.includes(cb.value);
      });
    }
  }

  modal.showModal();
}

function closeServiceModal() {
  const modal = $('#serviceModal');
  if (modal) modal.close();
  editingServiceId = null;
}

function handleTemplateChange(e) {
  const templateType = e.target.value;
  if (!templateType) return;

  // Templates use 'type' field from the backend
  const template = serviceTemplates.find(t => t.type === templateType);
  if (!template) return;

  // Auto-fill form fields from template
  $('#serviceName').value = template.name;
  $('#serviceCheckType').value = template.check_type;

  // Auto-fill icon URL from template if available
  if (template.icon_url) {
    $('#serviceIconUrl').value = template.icon_url;
    updateIconPreview(template.icon_url);
  }

  // Set URL placeholder based on template
  if (template.default_url) {
    const urlField = $('#serviceUrl');
    if (!urlField.value) {
      urlField.placeholder = template.default_url;
    }
  }

  // Show help text if available
  const helpEl = $('#templateHelp');
  if (helpEl && template.help_text) {
    helpEl.textContent = template.help_text;
  }

  // Show/hide token field based on whether it's required
  const tokenGroup = $('#tokenGroup');
  const tokenHelp = $('#tokenHelp');
  if (tokenGroup) {
    if (template.requires_token) {
      tokenGroup.classList.remove('hidden');
      if (tokenHelp && template.token_header) {
        tokenHelp.textContent = `Required header: ${template.token_header}`;
      }
    }
  }
}

// Test service connection before saving
async function testServiceConnection() {
  const url = $('#serviceUrl').value.trim();
  const apiToken = $('#serviceToken').value.trim();
  const checkType = $('#serviceCheckType').value;
  const timeout = parseInt($('#serviceTimeout').value) || 5;
  const serviceType = $('#serviceTemplate').value || $('#serviceType').value || 'custom';

  const resultEl = $('#testConnectionResult');
  const btn = $('#testServiceConnection');

  if (!url) {
    if (resultEl) {
      resultEl.textContent = 'Please enter a URL first';
      resultEl.className = 'test-result error';
      resultEl.classList.remove('hidden');
    }
    return;
  }

  // Show loading state
  if (btn) {
    btn.disabled = true;
    btn.textContent = 'Testing...';
  }
  if (resultEl) {
    resultEl.textContent = 'Testing connection...';
    resultEl.className = 'test-result';
    resultEl.classList.remove('hidden');
  }

  try {
    const payload = {
      url,
      api_token: apiToken,
      check_type: checkType,
      timeout,
      service_type: serviceType
    };
    // If editing and no new token entered, tell backend to use the stored token
    if (editingServiceId && !apiToken) {
      payload.service_id = editingServiceId;
    }

    const resp = await j('/api/admin/services/test', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCsrf()
      },
      body: JSON.stringify(payload)
    });

    if (resultEl) {
      if (resp.success) {
        let msg = '✓ Connection successful';
        if (resp.status_code) {
          msg += ` (${resp.status_code})`;
        }
        if (resp.latency_ms !== undefined) {
          msg += ` - ${resp.latency_ms}ms`;
        }
        resultEl.textContent = msg;
        resultEl.className = 'test-result success';
      } else {
        resultEl.textContent = '✗ ' + (resp.error || 'Connection failed');
        resultEl.className = 'test-result error';
      }
      resultEl.classList.remove('hidden');
    }
  } catch (e) {
    console.error('Connection test failed:', e);
    if (resultEl) {
      resultEl.textContent = '✗ ' + (e.body?.error || e.message || 'Connection test failed');
      resultEl.className = 'test-result error';
      resultEl.classList.remove('hidden');
    }
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = 'Test Connection';
    }
  }
}

async function saveService() {
  // Collect depends_on from checkboxes
  const dependsOnContainer = $('#serviceDependsOnList');
  const dependsOn = dependsOnContainer
    ? Array.from(dependsOnContainer.querySelectorAll('.depends-on-cb:checked')).map(cb => cb.value).join(',')
    : '';

  // Collect connected_to from checkboxes
  const connectedToContainer = $('#serviceConnectedToList');
  const connectedTo = connectedToContainer
    ? Array.from(connectedToContainer.querySelectorAll('.connected-to-cb:checked')).map(cb => cb.value).join(',')
    : '';

  const serviceData = {
    name: $('#serviceName').value.trim(),
    url: $('#serviceUrl').value.trim(),
    key: generateServiceKey($('#serviceName').value),
    service_type: $('#serviceTemplate').value || $('#serviceType').value || 'custom',
    api_token: $('#serviceToken').value.trim(),
    icon_url: $('#serviceIconUrl').value.trim(),
    check_type: $('#serviceCheckType').value,
    timeout: parseInt($('#serviceTimeout').value) || 5,
    check_interval: parseInt($('#serviceInterval').value) || 60,
    expected_min: parseInt($('#serviceExpectedMin').value) || 200,
    expected_max: parseInt($('#serviceExpectedMax').value) || 399,
    visible: $('#serviceVisible').checked,
    depends_on: dependsOn,
    connected_to: connectedTo
  };

  if (!serviceData.name || !serviceData.url) {
    const errEl = $('#serviceError');
    if (errEl) {
      errEl.textContent = 'Name and URL are required';
      errEl.classList.remove('hidden');
    }
    return;
  }

  try {
    if (editingServiceId) {
      // Update existing service
      await j(`/api/admin/services/${editingServiceId}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(serviceData)
      });
      showToast('Service updated');
    } else {
      // Create new service
      await j('/api/admin/services', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-CSRF-Token': getCsrf()
        },
        body: JSON.stringify(serviceData)
      });
      showToast('Service created');
    }

    closeServiceModal();
    loadAllServices();
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
      refresh();
    });
  } catch (e) {
    console.error('Failed to save service', e);
    const errEl = $('#serviceError');
    if (errEl) {
      errEl.textContent = e.body?.error || 'Failed to save service';
      errEl.classList.remove('hidden');
    }
  }
}

async function deleteService() {
  if (!editingServiceId) return;

  if (!confirm('Are you sure you want to delete this service? All monitoring data will be lost.')) {
    return;
  }

  try {
    await j(`/api/admin/services/${editingServiceId}`, {
      method: 'DELETE',
      headers: { 'X-CSRF-Token': getCsrf() }
    });
    showToast('Service deleted');
    closeServiceModal();
    loadAllServices();
    loadServices().then(services => {
      renderDynamicUptimeBars(services);
      refresh();
    });
  } catch (e) {
    console.error('Failed to delete service', e);
    showToast('Failed to delete service', 'error');
  }
}

function generateServiceKey(name) {
  return name.toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
    .substring(0, 32);
}

// Initialize services management
function initServicesManagement() {
  // Load templates
  loadServiceTemplates();

  // Add service button
  const addBtn = $('#addServiceBtn');
  if (addBtn) {
    addBtn.addEventListener('click', () => openServiceModal());
  }

  // Service modal handlers
  const closeBtn = $('#closeServiceModal');
  if (closeBtn) {
    closeBtn.addEventListener('click', closeServiceModal);
  }

  const cancelBtn = $('#cancelService');
  if (cancelBtn) {
    cancelBtn.addEventListener('click', closeServiceModal);
  }

  const saveBtn = $('#saveService');
  if (saveBtn) {
    saveBtn.addEventListener('click', saveService);
  }

  const testBtn = $('#testServiceConnection');
  if (testBtn) {
    testBtn.addEventListener('click', testServiceConnection);
  }

  const deleteBtn = $('#deleteService');
  if (deleteBtn) {
    deleteBtn.addEventListener('click', deleteService);
  }

  const templateSelect = $('#serviceTemplate');
  if (templateSelect) {
    templateSelect.addEventListener('change', handleTemplateChange);
  }

  // Update icon preview when URL changes
  const iconUrlInput = $('#serviceIconUrl');
  if (iconUrlInput) {
    iconUrlInput.addEventListener('input', (e) => {
      updateIconPreview(e.target.value.trim());
    });
  }

  // Close modal on backdrop click
  const modal = $('#serviceModal');
  if (modal) {
    modal.addEventListener('click', (e) => {
      if (e.target === modal) closeServiceModal();
    });
  }

  // Load services when Services tab is clicked
  const tabBtns = $$('.tab-btn');
  tabBtns.forEach(btn => {
    if (btn.getAttribute('data-tab') === 'services') {
      btn.addEventListener('click', loadAllServices);
    }
  });

  // Load admin services list immediately if the panel exists
  if ($('#servicesList')) {
    loadAllServices().catch(e => {
      console.warn('Initial admin services load failed (not logged in?):', e);
    });
  }
}
