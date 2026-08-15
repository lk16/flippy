// Stats table: boards per disc count (rows) and per search (columns). A column is the search a
// board actually got -- "32 @ 73%" is a 32-ply search with edax's 73% selectivity -- not the level
// it was requested at, since (disc count, level) determines both and different levels can mean the
// identical search. /api/stats already merges those (see api.statEntries).

// UNLEARNED_LABEL is the column for boards with no evaluation yet; the API reports them as depth 0.
const UNLEARNED_LABEL = 'unlearned';

// BOARD_SQUARES: a search whose depth plus the board's disc count reaches this searched to the end
// of the game, so its depth is that board's maximum. Those columns are labeled "max" and grouped
// together: the depth behind them differs per row, but "searched to the end" is one thing.
const BOARD_SQUARES = 64;

// columnFor returns the column a stat entry belongs in: key to group by, label to print, and a
// sort key ordering unlearned boards first, then searches by depth, then the searches that reached
// the end of the game -- each group by ascending confidence.
function columnFor(stat) {
    if (stat.depth === 0) {
        return { key: UNLEARNED_LABEL, label: UNLEARNED_LABEL, sort: [0, 0, 0] };
    }
    if (stat.depth + stat.disc_count === BOARD_SQUARES) {
        return { key: `max:${stat.confidence}`, label: `max @ ${stat.confidence}%`, sort: [2, 0, stat.confidence] };
    }
    return {
        key: `${stat.depth}:${stat.confidence}`,
        label: `${stat.depth} @ ${stat.confidence}%`,
        sort: [1, stat.depth, stat.confidence],
    };
}

// compareColumns orders two columns by their sort keys, lexicographically.
function compareColumns(a, b) {
    for (let i = 0; i < a.sort.length; i++) {
        if (a.sort[i] !== b.sort[i]) return a.sort[i] - b.sort[i];
    }
    return 0;
}

// buildStatsRows turns /api/stats entries into the table's cells: a header row, one row per disc
// count, and a totals row, each with a trailing total column.
function buildStatsRows(stats) {
    const countsByKey = new Map();
    const discCounts = new Set();
    const columns = new Map();

    stats.forEach((stat) => {
        const column = columnFor(stat);
        countsByKey.set(`${stat.disc_count}:${column.key}`, stat.count);
        discCounts.add(stat.disc_count);
        columns.set(column.key, column);
    });

    const sortedDiscCounts = [...discCounts].sort((a, b) => a - b);
    const sortedColumns = [...columns.values()].sort(compareColumns);

    const rows = [];

    const headerRow = [''];
    sortedColumns.forEach((column) => headerRow.push(column.label));
    headerRow.push('Total');
    rows.push(headerRow);

    const columnTotals = new Array(sortedColumns.length).fill(0);
    sortedDiscCounts.forEach((discCount) => {
        const row = [`${discCount} discs`];
        let rowTotal = 0;

        sortedColumns.forEach((column, colIndex) => {
            const count = countsByKey.get(`${discCount}:${column.key}`) || 0;
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

    return rows;
}

async function loadStats() {
    const response = await fetch('/api/stats');
    const stats = await response.json();

    renderTable(buildStatsRows(stats));
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

// In the browser there is no CommonJS `module`, so the page starts polling; under Node the pure
// table-building logic is exported for tests instead.
if (typeof module === 'undefined') {
    pollStats();
} else {
    module.exports = { buildStatsRows, columnFor };
}
