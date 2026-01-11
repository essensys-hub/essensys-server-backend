package api

const TableRefHTML = `<!DOCTYPE html>
<html lang="fr">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Essensys - Table de Référence</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; margin: 2rem; background: #f9f9f9; color: #333; }
        h1 { font-size: 1.5rem; margin-bottom: 1rem; color: #2c3e50; }
        .stats { margin-bottom: 1rem; color: #666; font-size: 0.9rem; }
        table { width: 100%; max-width: 800px; border-collapse: collapse; background: white; box-shadow: 0 1px 3px rgba(0,0,0,0.1); border-radius: 8px; overflow: hidden; }
        th, td { padding: 12px 16px; text-align: left; border-bottom: 1px solid #eee; }
        th { background: #f8f9fa; font-weight: 600; color: #444; text-transform: uppercase; font-size: 0.8rem; letter-spacing: 0.05em; }
        tr:last-child td { border-bottom: none; }
        tr:hover { background-color: #f1f1f1; }
        .key { font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace; color: #0969da; }
        .value { font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace; color: #0550ae; }
        .empty-state { padding: 2rem; text-align: center; color: #888; }
        .refresh-btn { display: inline-block; margin-top: 1rem; padding: 8px 16px; background: #0969da; color: white; text-decoration: none; border-radius: 6px; font-size: 0.9rem; }
        .refresh-btn:hover { background: #0550ae; }
    </style>
</head>
<body>
    <h1>Table de Référence (Redis)</h1>
    <div class="stats">
        <span>Table: {{ .Count }} entrées</span> | 
        <span>Queue: {{ .QueueCount }} actions en attente</span>
    </div>
    
    {{ if .Queue }}
    <h2>File d'Attente (Actions)</h2>
    <table>
        <thead>
            <tr>
                <th>GUID</th>
                <th>Params</th>
            </tr>
        </thead>
        <tbody>
            {{ range .Queue }}
            <tr>
                <td class="key">{{ .GUID }}</td>
                <td class="value">
                    {{ range .Params }}
                    {{ .K }}:{{ .V }} 
                    {{ end }}
                </td>
            </tr>
            {{ end }}
        </tbody>
    </table>
    <br>
    <h2>Table d'Échange</h2>
    {{ end }}
    
    {{ if .Items }}
    <table>
        <thead>
            <tr>
                <th>Index (Clé)</th>
                <th>Valeur</th>
            </tr>
        </thead>
        <tbody>
            {{ range .Items }}
            <tr>
                <td class="key">{{ .Key }}</td>
                <td class="value">{{ .Value }}</td>
            </tr>
            {{ end }}
        </tbody>
    </table>
    {{ else }}
    <div class="table-container">
        <div class="empty-state">Aucune donnée disponible dans la table d'échange.</div>
    </div>
    {{ end }}

    <a href="javascript:location.reload()" class="refresh-btn">Actualiser</a>
</body>
</html>`
