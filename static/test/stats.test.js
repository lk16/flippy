// Tests for the stats table's two header rows: three fixed groups (unlearned, partially learned,
// learned) with one column per below-target search under the middle one. The bucket and the row's
// target come from /api/stats (api.statEntries), so these tests feed the shapes the server sends.
const assert = require('node:assert');
const { test } = require('./framework');
const { buildStatsTable, partialColumns, searchLabel } = require('../stats');

// entry builds one /api/stats entry; target defaults to a 40 @ 73% search (the 12-disc target).
function entry({ discs, depth, confidence = 73, count, bucket, target = [40, 73] }) {
    return {
        disc_count: discs,
        depth,
        confidence: depth === 0 ? 0 : confidence,
        count,
        bucket,
        target_depth: target[0],
        target_confidence: target[1],
    };
}

// groupLabels returns the top header row as [label, span] pairs.
function groupLabels(table) {
    return table.groups.map((g) => [g.label, g.span]);
}

// The group row is rendered with colSpan, so a total that disagrees with the column count silently
// shears the table.
function assertSpansCoverColumns(table) {
    const span = table.groups.reduce((sum, g) => sum + g.span, 0);
    assert.equal(span, table.rows[0].length, 'group spans cover exactly the columns');
}

test('searchLabel: names a search by its depth and confidence', () => {
    assert.equal(searchLabel(32, 73), '32 @ 73%');
    assert.equal(searchLabel(41, 73), '41 @ 73%');
});

test('partialColumns: one column per below-target search, shallowest first', () => {
    const columns = partialColumns([
        entry({ discs: 12, depth: 32, count: 1, bucket: 'partial' }),
        entry({ discs: 12, depth: 16, count: 2, bucket: 'partial' }),
        entry({ discs: 12, depth: 32, confidence: 95, count: 3, bucket: 'partial' }),
        entry({ discs: 12, depth: 16, count: 4, bucket: 'partial' }), // same search, same column
        entry({ discs: 12, depth: 40, count: 5, bucket: 'learned' }),
        entry({ discs: 12, depth: 0, count: 6, bucket: 'unlearned' }),
    ]);

    assert.deepEqual(columns.map((c) => searchLabel(c.depth, c.confidence)), ['16 @ 73%', '32 @ 73%', '32 @ 95%']);
});

test('buildStatsTable: three groups, with one column per partial search between them', () => {
    const table = buildStatsTable([
        entry({ discs: 12, depth: 0, count: 1, bucket: 'unlearned' }),
        entry({ discs: 12, depth: 16, count: 2, bucket: 'partial' }),
        entry({ discs: 12, depth: 32, count: 3, bucket: 'partial' }),
        entry({ discs: 12, depth: 40, count: 4, bucket: 'learned' }),
    ]);

    assert.deepEqual(groupLabels(table), [['', 1], ['Unlearned', 1], ['Partially learned', 2], ['Learned', 2], ['', 1]]);
    assert.deepEqual(table.rows[0], ['', '0', '16 @ 73%', '32 @ 73%', 'target', 'count', 'Total']);
    assertSpansCoverColumns(table);
    assert.deepEqual(table.rows[1], ['12 discs', 1, 2, 3, '40 @ 73%', 4, 10]);
});

test('buildStatsTable: the target column shows each row\'s own target search', () => {
    const table = buildStatsTable([
        entry({ discs: 12, depth: 40, count: 1, bucket: 'learned', target: [40, 73] }),
        entry({ discs: 13, depth: 41, count: 2, bucket: 'learned', target: [41, 73] }),
        entry({ discs: 28, depth: 36, confidence: 95, count: 3, bucket: 'learned', target: [36, 95] }),
    ]);

    assert.deepEqual(table.rows.map((r) => r[r.length - 3]), ['target', '40 @ 73%', '41 @ 73%', '36 @ 95%', '']);
});

// The same search can be partial for one disc count and at target for another; only the partial
// cells get a column, so a learned cell never lands under "Partially learned".
test('buildStatsTable: a search partial for one row and at target for another splits by bucket', () => {
    const table = buildStatsTable([
        entry({ discs: 12, depth: 36, count: 7, bucket: 'partial', target: [40, 73] }),
        entry({ discs: 14, depth: 36, count: 5, bucket: 'learned', target: [36, 73] }),
    ]);

    assert.deepEqual(table.rows[0], ['', '0', '36 @ 73%', 'target', 'count', 'Total']);
    assert.deepEqual(table.rows[1], ['12 discs', 0, 7, '40 @ 73%', 0, 7]);
    assert.deepEqual(table.rows[2], ['14 discs', 0, 0, '36 @ 73%', 5, 5]);
});

test('buildStatsTable: rows sum across columns and columns across disc counts', () => {
    const table = buildStatsTable([
        entry({ discs: 12, depth: 40, count: 10, bucket: 'learned' }),
        entry({ discs: 13, depth: 41, count: 3, bucket: 'learned', target: [41, 73] }),
        entry({ discs: 13, depth: 0, count: 7, bucket: 'unlearned', target: [41, 73] }),
    ]);

    assert.deepEqual(table.rows[1], ['12 discs', 0, '40 @ 73%', 10, 10]);
    assert.deepEqual(table.rows[2], ['13 discs', 7, '41 @ 73%', 3, 10]);
    assert.deepEqual(table.rows[3], ['Total', 7, '', 13, 20]);
});

test('buildStatsTable: no entries yields the two header rows and an empty total', () => {
    const table = buildStatsTable([]);

    assert.deepEqual(groupLabels(table), [['', 1], ['Unlearned', 1], ['Learned', 2], ['', 1]]);
    assert.deepEqual(table.rows, [['', '0', 'target', 'count', 'Total'], ['Total', 0, '', 0, 0]]);
    assertSpansCoverColumns(table);
});
