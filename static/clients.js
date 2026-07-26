function formatTimestamp(isoString) {
    const date = new Date(isoString);
    const pad = (n) => String(n).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} `
        + `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

async function loadWorkers() {
    const response = await fetch('/api/workers');
    const workers = await response.json();

    const tbody = document.getElementById('worker-table-body');
    tbody.innerHTML = '';

    workers.forEach((worker) => {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${worker.id}</td>
            <td>${worker.hostname}</td>
            <td>${worker.git_commit.substring(0, 8)}</td>
            <td>${worker.positions_computed}</td>
            <td class="timestamp">${formatTimestamp(worker.last_active)}</td>
        `;
        tbody.appendChild(row);
    });
}

loadWorkers();
setInterval(loadWorkers, 1000);
