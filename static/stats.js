// Stats table: positions per disc count (rows), grouped by how far each row's evaluation got
// toward the target search for its disc count. A partial column is the search a position actually
// got -- "32 @ 73%" is a 32-ply search with edax's 73% selectivity -- not the level it was
// requested at, since (disc count, level) determines both and different levels can mean the
// identical search. /api/stats already merges those and classifies every entry (see
// api.statEntries), so this page needs no copy of edax's level table.

// Buckets, as api.statBucket labels them.
const BUCKET_UNLEARNED = 'unlearned';
const BUCKET_PARTIAL = 'partial';

// searchLabel names a search by the depth and confidence it ran at.
function searchLabel(depth, confidence) {
    return `${depth} @ ${confidence}%`;
}

// partialColumns returns the columns of the "Partially learned" group: every search seen below
// some row's target, ordered by depth and then confidence. One column can be partial for one disc
// count and already at target for another; only the partial cells land here.
function partialColumns(stats) {
    const columns = new Map();
    for (const stat of stats) {
        if (stat.bucket !== BUCKET_PARTIAL) continue;
        const key = `${stat.depth}:${stat.confidence}`;
        if (!columns.has(key)) {
            columns.set(key, { key, depth: stat.depth, confidence: stat.confidence });
        }
    }
    return [...columns.values()].sort((a, b) => a.depth - b.depth || a.confidence - b.confidence);
}

// buildStatsTable turns /api/stats entries into the table's contents: `groups` is the top header
// row, each entry spanning that many columns of `rows[0]` (the column header row); `rows` continues
// with one row per disc count and a totals row, each ending in a row total.
function buildStatsTable(stats) {
    const columns = partialColumns(stats);
    const discCounts = [...new Set(stats.map((s) => s.disc_count))].sort((a, b) => a - b);

    const groups = [{ label: '', span: 1 }, { label: 'Unlearned', span: 1 }];
    if (columns.length > 0) groups.push({ label: 'Partially learned', span: columns.length });
    groups.push({ label: 'Learned', span: 2 }, { label: '', span: 1 });

    const header = ['', '0', ...columns.map((c) => searchLabel(c.depth, c.confidence)), 'target', 'count', 'Total'];
    const rows = [header];

    // Counts per disc count, laid out as [unlearned, ...partials, learned]; the totals row then
    // falls out of the same accumulators. The target column holds a label, so it never gets a total.
    const learnedIndex = columns.length + 1;
    const partialIndex = new Map(columns.map((c, i) => [c.key, i + 1]));
    const columnTotals = new Array(columns.length + 2).fill(0);

    for (const discCount of discCounts) {
        const cells = new Array(columns.length + 2).fill(0);
        let target = '';

        for (const stat of stats) {
            if (stat.disc_count !== discCount) continue;
            target = searchLabel(stat.target_depth, stat.target_confidence);
            if (stat.bucket === BUCKET_UNLEARNED) {
                cells[0] += stat.count;
            } else if (stat.bucket === BUCKET_PARTIAL) {
                cells[partialIndex.get(`${stat.depth}:${stat.confidence}`)] += stat.count;
            } else {
                cells[learnedIndex] += stat.count;
            }
        }

        cells.forEach((count, i) => { columnTotals[i] += count; });
        const rowTotal = cells.reduce((sum, count) => sum + count, 0);
        rows.push([`${discCount} discs`, ...cells.slice(0, learnedIndex), target, cells[learnedIndex], rowTotal]);
    }

    const grandTotal = columnTotals.reduce((sum, count) => sum + count, 0);
    rows.push(['Total', ...columnTotals.slice(0, learnedIndex), '', columnTotals[learnedIndex], grandTotal]);

    return { groups, rows };
}

async function loadStats() {
    const response = await fetch('/api/stats');
    const stats = await response.json();

    renderTable(buildStatsTable(stats));
}

function renderTable({ groups, rows }) {
    const table = document.getElementById('stats-table');
    table.innerHTML = '';

    const groupRow = document.createElement('tr');
    groups.forEach((group) => {
        const th = document.createElement('th');
        th.textContent = group.label;
        th.colSpan = group.span;
        if (group.label) th.className = 'group';
        groupRow.appendChild(th);
    });
    table.appendChild(groupRow);

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
    module.exports = { buildStatsTable, partialColumns, searchLabel };
}
