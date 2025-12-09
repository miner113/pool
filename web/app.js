// Juno Cash Pool Dashboard
const API_BASE = '/api';

// Utility functions
function formatHashrate(h) {
    if (!h || h === 0) return '0 H/s';
    if (h >= 1e12) return (h / 1e12).toFixed(2) + ' TH/s';
    if (h >= 1e9) return (h / 1e9).toFixed(2) + ' GH/s';
    if (h >= 1e6) return (h / 1e6).toFixed(2) + ' MH/s';
    if (h >= 1e3) return (h / 1e3).toFixed(2) + ' KH/s';
    return h.toFixed(2) + ' H/s';
}

function formatJuno(satoshis) {
    if (!satoshis) return '0 JUNO';
    return (satoshis / 1e8).toFixed(8) + ' JUNO';
}

function formatTime(timestamp) {
    if (!timestamp) return '--';
    const date = new Date(timestamp);
    return date.toLocaleString();
}

function formatTimeAgo(timestamp) {
    if (!timestamp) return '--';
    const date = new Date(timestamp);
    const now = new Date();
    const diff = Math.floor((now - date) / 1000);
    
    if (diff < 60) return diff + 's ago';
    if (diff < 3600) return Math.floor(diff / 60) + 'm ago';
    if (diff < 86400) return Math.floor(diff / 3600) + 'h ago';
    return Math.floor(diff / 86400) + 'd ago';
}

function truncateHash(hash, len = 8) {
    if (!hash) return '--';
    if (hash.length <= len * 2) return hash;
    return hash.substring(0, len) + '...' + hash.substring(hash.length - len);
}

function formatDifficulty(d) {
    if (!d || d === 0) return '0';
    if (d >= 1e12) return (d / 1e12).toFixed(2) + 'T';
    if (d >= 1e9) return (d / 1e9).toFixed(2) + 'G';
    if (d >= 1e6) return (d / 1e6).toFixed(2) + 'M';
    if (d >= 1e3) return (d / 1e3).toFixed(2) + 'K';
    return d.toFixed(4);
}

// API calls
async function fetchAPI(endpoint) {
    try {
        const response = await fetch(`${API_BASE}${endpoint}`);
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return await response.json();
    } catch (error) {
        console.error(`API Error (${endpoint}):`, error);
        return null;
    }
}

// Load pool stats
async function loadPoolStats() {
    const data = await fetchAPI('/pool/stats');
    if (data) {
        document.getElementById('pool-hashrate').textContent = formatHashrate(data.total_hashrate);
        document.getElementById('miners-online').textContent = data.connected_miners || 0;
        document.getElementById('blocks-found').textContent = data.blocks_found || 0;
        document.getElementById('pool-fee').textContent = (data.pool_fee_percent || 1) + '%';
    }
}

// Load network stats
async function loadNetworkStats() {
    const data = await fetchAPI('/network/stats');
    if (data) {
        document.getElementById('network-hashrate').textContent = formatHashrate(data.network_hashrate);
        document.getElementById('block-height').textContent = data.block_height || 0;
        document.getElementById('difficulty').textContent = formatDifficulty(data.difficulty);
    }
}

// Load blocks
async function loadBlocks() {
    const tbody = document.getElementById('blocks-table');
    const data = await fetchAPI('/pool/blocks?limit=20');
    
    if (!data || !data.blocks || data.blocks.length === 0) {
        tbody.innerHTML = '<tr><td colspan="5" class="loading">&gt;no_blocks_yet</td></tr>';
        return;
    }
    
    tbody.innerHTML = data.blocks.map(block => `
        <tr>
            <td>${block.height}</td>
            <td class="hash">${truncateHash(block.hash, 10)}</td>
            <td><span class="status status-${block.status}">${block.status}</span></td>
            <td>${block.confirmations || 0}</td>
            <td>${formatTimeAgo(block.found_at)}</td>
        </tr>
    `).join('');
}

// Load payouts
async function loadPayouts() {
    const tbody = document.getElementById('payouts-table');
    const data = await fetchAPI('/pool/payments?limit=20');
    
    if (!data || !data.payments || data.payments.length === 0) {
        tbody.innerHTML = '<tr><td colspan="4" class="loading">&gt;no_payouts_yet</td></tr>';
        return;
    }
    
    tbody.innerHTML = data.payments.map(payout => `
        <tr>
            <td class="hash">${payout.txid ? truncateHash(payout.txid, 10) : '--'}</td>
            <td>${formatJuno(payout.amount)}</td>
            <td><span class="status status-${payout.status}">${payout.status}</span></td>
            <td>${formatTimeAgo(payout.timestamp)}</td>
        </tr>
    `).join('');
}

// Miner lookup
async function lookupMiner() {
    const address = document.getElementById('miner-address').value.trim();
    if (!address) return;
    
    const statsDiv = document.getElementById('miner-stats');
    const data = await fetchAPI(`/miner/lookup?address=${encodeURIComponent(address)}`);
    
    if (!data || data.error) {
        statsDiv.classList.add('hidden');
        alert('Miner not found');
        return;
    }
    
    document.getElementById('miner-addr').textContent = truncateHash(data.username || address, 12);
    document.getElementById('miner-hashrate').textContent = formatHashrate(data.hashrate);
    document.getElementById('miner-balance').textContent = formatJuno(data.balance);
    document.getElementById('miner-paid').textContent = formatJuno(data.total_paid);
    
    statsDiv.classList.remove('hidden');
}

// Allow Enter key for lookup
document.addEventListener('DOMContentLoaded', () => {
    const input = document.getElementById('miner-address');
    if (input) {
        input.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') lookupMiner();
        });
    }
});

// Load all data
async function loadAll() {
    await Promise.all([
        loadPoolStats(),
        loadNetworkStats(),
        loadBlocks(),
        loadPayouts()
    ]);
}

// Initial load
loadAll();

// Refresh every 15 seconds
setInterval(loadAll, 15000);

// Smooth scroll for nav links
document.querySelectorAll('nav a').forEach(link => {
    link.addEventListener('click', (e) => {
        const href = link.getAttribute('href');
        if (href.startsWith('#')) {
            e.preventDefault();
            const target = document.querySelector(href);
            if (target) {
                target.scrollIntoView({ behavior: 'smooth' });
            }
        }
    });
});
