async function loadStats() {
    const response = await fetch('/api/stats');
    const stats = await response.json();

    const countsByKey = new Map();
    const discCounts = new Set();
    const levels = new Set();

    stats.forEach((stat) => {
        countsByKey.set(`${stat.disc_count}:${stat.level}`, stat.count);
        discCounts.add(stat.disc_count);
        levels.add(stat.level);
    });

    const sortedDiscCounts = [...discCounts].sort((a, b) => a - b);
    const sortedLevels = [...levels].sort((a, b) => a - b);

    const rows = [];

    const headerRow = [''];
    sortedLevels.forEach((level) => headerRow.push(`Level ${level}`));
    headerRow.push('Total');
    rows.push(headerRow);

    const columnTotals = new Array(sortedLevels.length).fill(0);
    sortedDiscCounts.forEach((discCount) => {
        const row = [`${discCount} discs`];
        let rowTotal = 0;

        sortedLevels.forEach((level, colIndex) => {
            const count = countsByKey.get(`${discCount}:${level}`) || 0;
            row.push(count);
            columnTotals[colIndex] += count;
            rowTotal += count;
        });

        row.push(rowTotal);
        rows.push(row);
    });

    const totalsRow = ['Total'];
    let grandTotal = 0;
    columnTotals.forEach((total) => {
        totalsRow.push(total);
        grandTotal += total;
    });
    totalsRow.push(grandTotal);
    rows.push(totalsRow);

    renderTable(rows);
}

function renderTable(rows) {
    const table = document.getElementById('stats-table');
    table.innerHTML = '';

    rows.forEach((row, rowIndex) => {
        const tr = document.createElement('tr');

        row.forEach((cell, colIndex) => {
            const td = document.createElement(rowIndex === 0 || colIndex === 0 ? 'th' : 'td');

            if (cell === 0) {
                td.textContent = '';
            } else {
                td.textContent = cell;
                if (typeof cell === 'number') {
                    td.className = 'numeric';
                }
            }

            if (rowIndex === rows.length - 1 || colIndex === row.length - 1) {
                td.classList.add('total');
            }

            tr.appendChild(td);
        });

        table.appendChild(tr);
    });
}

// Poll by re-scheduling only after each load settles, so a load slower than
// the interval can't stack up overlapping requests (and a failed load doesn't
// stop the polling).
async function pollStats() {
    try {
        await loadStats();
    } catch (error) {
        console.error('Error loading stats:', error);
    }
    setTimeout(pollStats, 1000);
}

pollStats();
