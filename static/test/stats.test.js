// Tests for the stats table's columns: one per search rather than per level, labeled with the
// depth and confidence the search ran at, ordered shallow -> deep -> "searched to the end".
const assert = require('node:assert');
const { test } = require('./framework');
const { buildStatsRows, columnFor } = require('../stats');

// headerOf returns the column labels of a built table, without the leading blank and the "Total".
function headerOf(rows) {
    return rows[0].slice(1, -1);
}

test('columnFor: labels a search by its depth and confidence', () => {
    assert.equal(columnFor({ disc_count: 21, depth: 32, confidence: 73 }).label, '32 @ 73%');
    assert.equal(columnFor({ disc_count: 17, depth: 34, confidence: 73 }).label, '34 @ 73%');
});

test('columnFor: labels a search that reached the end of the game "max"', () => {
    // 25 discs + 39 depth and 28 discs + 36 depth both run out the game, at different depths.
    assert.equal(columnFor({ disc_count: 25, depth: 39, confidence: 73 }).label, 'max @ 73%');
    assert.equal(columnFor({ disc_count: 28, depth: 36, confidence: 73 }).label, 'max @ 73%');
    assert.equal(columnFor({ disc_count: 44, depth: 20, confidence: 100 }).label, 'max @ 100%');
});

test('columnFor: boards that were never searched get their own column', () => {
    assert.equal(columnFor({ disc_count: 12, depth: 0, confidence: 0 }).label, 'unlearned');
});

test('buildStatsRows: columns run unlearned, then by depth, then confidence, with max last', () => {
    const rows = buildStatsRows([
        { disc_count: 21, depth: 43, confidence: 100, count: 1 }, // max, full width
        { disc_count: 21, depth: 32, confidence: 95, count: 2 },
        { disc_count: 21, depth: 32, confidence: 73, count: 3 },
        { disc_count: 21, depth: 0, confidence: 0, count: 4 },
        { disc_count: 21, depth: 43, confidence: 73, count: 5 }, // max, selective
        { disc_count: 21, depth: 20, confidence: 73, count: 6 },
    ]);

    assert.deepEqual(headerOf(rows), ['unlearned', '20 @ 73%', '32 @ 73%', '32 @ 95%', 'max @ 73%', 'max @ 100%']);
});

test('buildStatsRows: rows sum across columns and columns across disc counts', () => {
    const rows = buildStatsRows([
        { disc_count: 12, depth: 40, confidence: 73, count: 10 },
        { disc_count: 13, depth: 40, confidence: 73, count: 3 },
        { disc_count: 13, depth: 0, confidence: 0, count: 7 },
    ]);

    assert.deepEqual(headerOf(rows), ['unlearned', '40 @ 73%']);
    assert.deepEqual(rows[1], ['12 discs', 0, 10, 10]);
    assert.deepEqual(rows[2], ['13 discs', 7, 3, 10]);
    assert.deepEqual(rows[3], ['Total', 7, 13, 20]);
});

test('buildStatsRows: one "max" column can hold different depths from different rows', () => {
    // A 25-disc board solved to the end searches 39 ply, a 28-disc one 36 -- the same column.
    const rows = buildStatsRows([
        { disc_count: 25, depth: 39, confidence: 73, count: 2 },
        { disc_count: 28, depth: 36, confidence: 73, count: 5 },
    ]);

    assert.deepEqual(headerOf(rows), ['max @ 73%']);
    assert.deepEqual(rows[1], ['25 discs', 2, 2]);
    assert.deepEqual(rows[2], ['28 discs', 5, 5]);
    assert.deepEqual(rows[3], ['Total', 7, 7]);
});

test('buildStatsRows: no entries yields just the header and an empty total', () => {
    assert.deepEqual(buildStatsRows([]), [['', 'Total'], ['Total', 0]]);
});
