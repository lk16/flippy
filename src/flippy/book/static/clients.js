function updateTable() {
    fetch('/stats')
        .then(response => response.json())
        .then(data => {
            const tbody = document.getElementById('clientTableBody');
            tbody.innerHTML = '';

            data.client_stats.forEach(client => {
                const date = new Date(client.last_active);
                const formattedDate = date.getFullYear() + '-' +
                    String(date.getMonth() + 1).padStart(2, '0') + '-' +
                    String(date.getDate()).padStart(2, '0') + ' ' +
                    String(date.getHours()).padStart(2, '0') + ':' +
                    String(date.getMinutes()).padStart(2, '0') + ':' +
                    String(date.getSeconds()).padStart(2, '0');

                const row = document.createElement('tr');
                row.innerHTML = `
                    <td>${client.id}</td>
                    <td>${client.hostname}</td>
                    <td>${client.git_commit.substring(0, 8)}</td>
                    <td>${client.positions_computed}</td>
                    <td class="timestamp">${formattedDate}</td>
                `;
                tbody.appendChild(row);
            });
        })
        .catch(error => console.error('Error fetching data:', error));
}

// Update immediately and then every second
updateTable();
setInterval(updateTable, 1000);
