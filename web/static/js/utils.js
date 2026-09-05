// Shared utility functions
window.getCsrf = function() {
    return (document.cookie.split('; ').find(s => s.startsWith('csrf=')) || '').split('=')[1] || '';
};

window.showToast = function(message, type = 'success') {
    // Remove any existing toast
    const existing = document.querySelector('.toast');
    if (existing) {
        existing.remove();
    }

    // Create and add the new toast
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = message;
    document.body.appendChild(toast);

    // Force a reflow to ensure animation works
    toast.offsetHeight;

    // Show the toast
    requestAnimationFrame(() => {
        toast.classList.add('show');
    });

    // Hide and remove the toast after delay
    setTimeout(() => {
        toast.classList.remove('show');
        setTimeout(() => toast.remove(), 300);
    }, 3000);
};

// HTML escaping — used across public and admin bundles
function escapeHtml(str) {
  // Quotes must be escaped too: callers also interpolate into HTML attributes.
  return String(str ?? '').replace(/[&<>"']/g, ch => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[ch]));
}
