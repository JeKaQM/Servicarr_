// IP Blocks management
function setupBlocksAdmin() {
    const adminPanel = document.getElementById('adminPanel');
    if (!adminPanel) return;

    async function loadBlocks() {
        const blocksList = document.getElementById('blocksList');
        if (!blocksList) return; // Tab not visible yet
        
        try {
            blocksList.innerHTML = '<div class="muted">Loading...</div>';
            
            const res = await fetch('/api/admin/blocks', {
                credentials: 'include',
                headers: {
                    'X-CSRF-Token': getCsrf()
                }
            });
            if (res.status === 401) {
                blocksList.innerHTML = '<div class="muted">Please log in to view blocked IPs</div>';
                return;
            }
            if (!res.ok) {
                throw new Error('Failed to load blocks');
            }
            const data = await res.json();
            const blocks = data.blocks;
            
            blocksList.innerHTML = '';
            
            if (!blocks || blocks.length === 0) {
                blocksList.innerHTML = '<div class="empty-state">No blocked IPs</div>';
                return;
            }

            blocks.forEach(block => {
                const expires = new Date(block.expires_at);
                const item = document.createElement('div');
                item.className = 'block-item';
                item.innerHTML = `
                    <div class="block-info">
                        <strong>${block.ip}</strong>
                        <span class="muted">Attempts: ${block.attempts}</span>
                        <span class="muted">Expires: ${expires.toLocaleString()}</span>
                    </div>
                    <button class="btn mini unblock" data-ip="${block.ip}">Unblock</button>
                `;
                blocksList.appendChild(item);
            });

            // Setup unblock buttons
            blocksList.querySelectorAll('.unblock').forEach(btn => {
                btn.onclick = async (e) => {
                    e.preventDefault();
                    try {
                        const ip = btn.dataset.ip;
                        btn.disabled = true;
                        const res = await fetch('/api/admin/unblock', {
                            method: 'POST',
                            credentials: 'include',
                            headers: {
                                'Content-Type': 'application/json',
                                'X-CSRF-Token': getCsrf()
                            },
                            body: JSON.stringify({ip})
                        });
                        if (!res.ok) {
                            throw new Error('Failed to unblock IP');
                        }
                        showToast(`Successfully unblocked ${ip}`);
                        await loadBlocks();
                    } catch (err) {
                        console.error('Unblock failed:', err);
                        showToast('Failed to unblock IP', 'error');
                        btn.disabled = false;
                    }
                };
            });
        } catch (err) {
            console.error('Loading blocks failed:', err);
            blocksList.innerHTML = '<div class="error-state">Failed to load blocked IPs</div>';
        }
    }

    // Setup reset all button
    const resetBtn = document.getElementById('resetBlocks');
    if (resetBtn) {
        resetBtn.onclick = async (e) => {
            e.preventDefault();
            const confirmResult = confirm('Are you sure you want to clear all IP blocks? This cannot be undone.');
            if (!confirmResult) return;
            
            try {
                resetBtn.disabled = true;
                resetBtn.textContent = 'Clearing...';
                
                const res = await fetch('/api/admin/clear-blocks', {
                    method: 'POST',
                    credentials: 'include',
                    headers: {
                        'X-CSRF-Token': getCsrf()
                    }
                });

                if (!res.ok) {
                    throw new Error('Failed to clear blocks');
                }
                
                const data = await res.json();
                const message = data.message || 'Successfully cleared all IP blocks';
                showToast(message);
                
                if (typeof showToast === 'undefined') {
                    console.error('showToast function is not defined!');
                    // Fallback toast implementation
                    const toast = document.createElement('div');
                    toast.className = 'toast success';
                    toast.textContent = message;
                    document.body.appendChild(toast);
                    setTimeout(() => toast.classList.add('show'), 10);
                    setTimeout(() => {
                        toast.classList.remove('show');
                        setTimeout(() => toast.remove(), 300);
                    }, 3000);
                } else {
                    showToast(message);
                }
                
                await loadBlocks();
            } catch (err) {
                console.error('Clear blocks failed:', err);
                alert('Failed to clear IP blocks: ' + (err.message || 'Unknown error'));
                
                // Error toast with debug
                if (typeof showToast === 'undefined') {
                    console.error('showToast function is not defined in error handler!');
                    // Fallback toast implementation
                    const toast = document.createElement('div');
                    toast.className = 'toast error';
                    toast.textContent = 'Failed to clear IP blocks: ' + (err.message || 'Unknown error');
                    document.body.appendChild(toast);
                    setTimeout(() => toast.classList.add('show'), 10);
                    setTimeout(() => {
                        toast.classList.remove('show');
                        setTimeout(() => toast.remove(), 300);
                    }, 3000);
                } else {
                    showToast('Failed to clear IP blocks: ' + (err.message || 'Unknown error'), 'error');
                }
            } finally {
                resetBtn.disabled = false;
                resetBtn.textContent = 'Clear All Blocks';
            }
        };
    }

    // Initial load
    loadBlocks();
    // Refresh every 30 seconds
    const intervalId = setInterval(loadBlocks, 30000);

    // Clean up interval when admin panel is hidden
    const observer = new MutationObserver((mutations) => {
        mutations.forEach((mutation) => {
            if (mutation.target.classList.contains('hidden')) {
                clearInterval(intervalId);
            }
        });
    });
    observer.observe(adminPanel, { attributes: true });
}

// Initialize when login state changes
document.addEventListener('loginStateChanged', setupBlocksAdmin);
// Also initialize on page load if already logged in
document.addEventListener('DOMContentLoaded', setupBlocksAdmin);