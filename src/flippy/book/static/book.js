async function loadStats() {
    const response = await fetch('/api/positions/stats');
    const data = await response.json();

    const table = document.getElementById('statsTable');
    // Clear existing rows
    table.innerHTML = '';

    data.forEach(row => {
        const tr = document.createElement('tr');
        row.forEach(cell => {
            const td = document.createElement(row === data[0] ? 'th' : 'td');
            if (cell === 0 || cell === '0') {
                td.textContent = '';
            } else {
                td.textContent = cell;
                if (!isNaN(cell) && typeof cell !== 'boolean') {
                    td.className = 'numeric';
                }
            }
            tr.appendChild(td);
        });
        table.appendChild(tr);
    });
}

// Update immediately and then every second
loadStats();
setInterval(loadStats, 1000);
