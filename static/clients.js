function formatTimestamp(isoString) {
    const date = new Date(isoString);
    const pad = (n) => String(n).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} `
        + `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

// createCell returns a <td> with text set via textContent, so worker-supplied
// values (id, hostname, git_commit) can't inject HTML into the page.
function createCell(text, className) {
    const td = document.createElement('td');
    td.textContent = text;
    if (className) td.className = className;
    return td;
}

async function loadWorkers() {
    const response = await fetch('/api/workers');
    const workers = await response.json();

    const tbody = document.getElementById('worker-table-body');
    tbody.innerHTML = '';

    workers.forEach((worker) => {
        const row = document.createElement('tr');
        row.appendChild(createCell(worker.id));
        row.appendChild(createCell(worker.hostname));
        row.appendChild(createCell((worker.git_commit || '').substring(0, 8)));
        row.appendChild(createCell(worker.positions_computed));
        row.appendChild(createCell(formatTimestamp(worker.last_active), 'timestamp'));
        tbody.appendChild(row);
    });
}

// Poll by re-scheduling only after each load settles, so a load slower than
// the interval can't stack up overlapping requests (and a failed load doesn't
// stop the polling).
async function pollWorkers() {
    try {
        await loadWorkers();
    } catch (error) {
        console.error('Error loading workers:', error);
    }
    setTimeout(pollWorkers, 1000);
}

pollWorkers();
