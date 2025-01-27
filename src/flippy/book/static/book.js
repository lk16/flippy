async function loadStats() {
    const response = await fetch('/api/stats/book');
    const data = await response.json();

    const table = document.getElementById('statsTable');
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

loadStats();
