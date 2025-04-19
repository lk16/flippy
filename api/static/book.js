async function loadStats() {
    const response = await fetch('/api/positions/stats');
    const stats = await response.json();

    // Create a map to store counts by disc_count and level
    const statsMap = new Map();
    const discCounts = new Set();
    const levels = new Set();

    stats.forEach(stat => {
        const key = `${stat.disc_count}:${stat.level}`;
        statsMap.set(key, stat.count);
        discCounts.add(stat.disc_count);
        levels.add(stat.level);
    });

    // Convert sets to sorted arrays
    const sortedDiscCounts = Array.from(discCounts).sort((a, b) => a - b);
    const sortedLevels = Array.from(levels).sort((a, b) => a - b);

    // Create the table data
    const tableData = [];

    // Create header row
    const headerRow = [''];
    sortedLevels.forEach(level => headerRow.push(`Level ${level}`));
    headerRow.push('Total');
    tableData.push(headerRow);

    // Create data rows and calculate row totals
    const columnTotals = new Array(sortedLevels.length).fill(0);
    sortedDiscCounts.forEach(discCount => {
        const row = [`${discCount} discs`];
        let rowTotal = 0;

        sortedLevels.forEach((level, colIndex) => {
            const count = statsMap.get(`${discCount}:${level}`) || 0;
            row.push(count);
            columnTotals[colIndex] += count;
            rowTotal += count;
        });

        row.push(rowTotal);
        tableData.push(row);
    });

    // Add totals row
    const totalsRow = ['Total'];
    let grandTotal = 0;
    sortedLevels.forEach((_, colIndex) => {
        totalsRow.push(columnTotals[colIndex]);
        grandTotal += columnTotals[colIndex];
    });
    totalsRow.push(grandTotal);
    tableData.push(totalsRow);

    // Render the table
    const table = document.getElementById('statsTable');
    table.innerHTML = '';

    tableData.forEach((row, rowIndex) => {
        const tr = document.createElement('tr');
        row.forEach((cell, colIndex) => {
            const td = document.createElement(rowIndex === 0 || colIndex === 0 ? 'th' : 'td');
            if (cell === 0 || cell === '0') {
                td.textContent = '';
            } else {
                td.textContent = cell;
                if (!isNaN(cell) && typeof cell !== 'boolean') {
                    td.className = 'numeric';
                }
            }
            // Add total styling to last row and column
            if (rowIndex === tableData.length - 1 || colIndex === row.length - 1) {
                td.classList.add('total');
            }
            tr.appendChild(td);
        });
        table.appendChild(tr);
    });
}

// Update immediately and then every second
loadStats();
setInterval(loadStats, 1000);
