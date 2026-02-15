document.addEventListener('DOMContentLoaded', function() {
    // Parse URL parameters
    const params = new URLSearchParams(window.location.search);
    const expiresAt = params.get('expires');
    
    function updateCountdown() {
        const countdownEl = document.getElementById('countdown');
        const retryBtn = document.getElementById('retryBtn');
        
        if (!expiresAt) {
            countdownEl.textContent = '24:00:00';
            return;
        }
        
        const expiry = new Date(expiresAt).getTime();
        const now = Date.now();
        const remaining = expiry - now;
        
        if (remaining <= 0) {
            countdownEl.textContent = 'Expired!';
            countdownEl.classList.add('countdown-expired');
            retryBtn.classList.remove('hidden');
            return;
        }
        
        const hours = Math.floor(remaining / (1000 * 60 * 60));
        const minutes = Math.floor((remaining % (1000 * 60 * 60)) / (1000 * 60));
        const seconds = Math.floor((remaining % (1000 * 60)) / 1000);
        
        countdownEl.textContent = 
            String(hours).padStart(2, '0') + ':' +
            String(minutes).padStart(2, '0') + ':' +
            String(seconds).padStart(2, '0');
    }
    
    // Update countdown every second
    updateCountdown();
    setInterval(updateCountdown, 1000);
    
    async function attemptUnblock() {
        const token = document.getElementById('unblockToken').value.trim();
        const resultEl = document.getElementById('unblockResult');
        const btn = document.getElementById('unblockBtn');
        
        if (!token) {
            resultEl.textContent = 'Please enter a recovery token';
            resultEl.className = 'unblock-result error';
            resultEl.classList.remove('hidden');
            return;
        }
        
        btn.disabled = true;
        btn.textContent = 'Verifying...';
        
        try {
            const resp = await fetch('/api/self-unblock', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ token: token })
            });
            
            const data = await resp.json();
            
            if (resp.ok && data.ok) {
                resultEl.textContent = data.message || 'Successfully unblocked! Redirecting...';
                resultEl.className = 'unblock-result success';
                resultEl.classList.remove('hidden');
                setTimeout(() => {
                    window.location.href = '/';
                }, 1500);
            } else {
                resultEl.textContent = data.message || 'Failed to unblock. Check your token.';
                resultEl.className = 'unblock-result error';
                resultEl.classList.remove('hidden');
                btn.disabled = false;
                btn.textContent = 'Unblock';
            }
        } catch (err) {
            resultEl.textContent = 'Network error. Please try again.';
            resultEl.className = 'unblock-result error';
            resultEl.classList.remove('hidden');
            btn.disabled = false;
            btn.textContent = 'Unblock';
        }
    }
    
    // Bind unblock button
    const unblockBtn = document.getElementById('unblockBtn');
    if (unblockBtn) {
        unblockBtn.addEventListener('click', attemptUnblock);
    }
    
    // Allow Enter key to submit
    const tokenInput = document.getElementById('unblockToken');
    if (tokenInput) {
        tokenInput.addEventListener('keypress', function(e) {
            if (e.key === 'Enter') {
                attemptUnblock();
            }
        });
    }
}); // End DOMContentLoaded
