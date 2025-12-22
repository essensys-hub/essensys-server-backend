import React, { useEffect, useState } from 'react';
import { getClients, startSimulation, getScenarios, stopSimulation, stopClient } from '../api';
import type { Emulator } from '../api';

interface ClientListProps {
    onSelectClient: (id: string) => void;
}

const ClientList: React.FC<ClientListProps> = ({ onSelectClient }) => {
    const [clients, setClients] = useState<Emulator[]>([]);
    const [count, setCount] = useState(1);
    const [serverIP, setServerIP] = useState('localhost');
    const [serverPort, setServerPort] = useState('8090');

    // Scenarios
    const [savedScenarios, setSavedScenarios] = useState<string[]>([]);
    const [startupScenario, setStartupScenario] = useState("");

    const fetchClients = async () => {
        try {
            const data = await getClients();
            setClients(data);
        } catch (e: any) {
            console.error("Failed to fetch clients", e);
            // Temporary debug alert
            alert("Debug: Client List Fetch Error: " + e.message);
        }
    };

    const fetchScenarios = async () => {
        try {
            const list = await getScenarios();
            setSavedScenarios(list || []);
        } catch (e) {
            console.error("Failed to fetch scenarios", e);
        }
    };

    useEffect(() => {
        fetchClients();
        fetchScenarios();
        const interval = setInterval(fetchClients, 2000);
        return () => clearInterval(interval);
    }, []);

    const handleStart = async () => {
        try {
            await startSimulation(count, serverIP, serverPort, startupScenario);
            alert(`Started batch of ${count} clients connecting to ${serverIP}:${serverPort} with scenario: ${startupScenario || 'None'}`);
            fetchClients();
        } catch (e: any) {
            alert("Failed to start simulation: " + (e.response?.data || e.message));
        }
    };

    const handleAddSingle = async () => {
        try {
            await startSimulation(1, serverIP, serverPort, startupScenario);
            // No alert for single add to keep it quick, or maybe small toast?
            // alert("Added 1 client"); 
            fetchClients();
        } catch (e: any) {
            alert("Failed to add client: " + (e.response?.data || e.message));
        }
    };

    return (
        <div className="p-4">
            <div className="mb-4 bg-gray-800 p-4 rounded-lg flex flex-wrap items-center gap-4 border border-gray-700">
                <h2 className="text-xl font-bold text-white mr-4">Fleet Control</h2>

                <div className="flex items-center gap-2">
                    <label className="text-sm text-gray-400">Server:</label>
                    <input
                        type="text"
                        value={serverIP}
                        onChange={(e) => setServerIP(e.target.value)}
                        className="p-2 rounded bg-gray-700 text-white border border-gray-600 w-32 focus:outline-none focus:border-blue-500"
                        placeholder="localhost"
                    />
                    <span className="text-gray-400">:</span>
                    <input
                        type="text"
                        value={serverPort}
                        onChange={(e) => setServerPort(e.target.value)}
                        className="p-2 rounded bg-gray-700 text-white border border-gray-600 w-20 focus:outline-none focus:border-blue-500"
                        placeholder="80"
                    />
                </div>

                <div className="h-8 w-px bg-gray-600 mx-2"></div>

                <div className="flex items-center gap-2">
                    <label className="text-sm text-gray-400">Startup Scen.:</label>
                    <select
                        value={startupScenario}
                        onChange={(e) => setStartupScenario(e.target.value)}
                        className="p-2 rounded bg-gray-700 text-white border border-gray-600 w-40 focus:outline-none focus:border-blue-500 text-sm"
                    >
                        <option value="">(None)</option>
                        {savedScenarios?.map(s => <option key={s} value={s}>{s}</option>)}
                    </select>
                </div>

                <div className="h-8 w-px bg-gray-600 mx-2"></div>

                <div className="flex items-center gap-2">
                    <input
                        type="number"
                        value={count}
                        onChange={(e) => setCount(Number(e.target.value))}
                        className="p-2 rounded bg-gray-700 text-white border border-gray-600 w-20 focus:outline-none focus:border-blue-500"
                        min="1" max="100"
                    />
                    <button
                        onClick={handleStart}
                        className="bg-blue-600 hover:bg-blue-500 text-white px-4 py-2 rounded transition-colors font-semibold shadow-lg"
                    >
                        Start Batch
                    </button>
                    <button
                        onClick={async () => {
                            if (!confirm("Stop all clients?")) return;
                            try {
                                await stopSimulation();
                                fetchClients();
                            } catch (e: any) {
                                alert("Failed to stop simulation: " + e.message);
                            }
                        }}
                        className="bg-red-600 hover:bg-red-500 text-white px-4 py-2 rounded transition-colors font-semibold shadow-lg"
                    >
                        Stop All
                    </button>
                </div>

                <button
                    onClick={handleAddSingle}
                    className="bg-green-600 hover:bg-green-500 text-white px-4 py-2 rounded transition-colors font-semibold shadow-lg ml-auto"
                >
                    + Add 1 Client
                </button>
            </div>

            <div className="flex justify-between items-center mb-4 px-2">
                <span className="text-gray-400 text-sm">Total Active Clients: <span className="text-white font-bold">{clients.length}</span></span>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {clients.map(client => (
                    <div
                        key={client.ID}
                        onClick={() => onSelectClient(client.ID)}
                        className="bg-gray-800 border border-gray-700 rounded-lg p-4 shadow-lg hover:border-blue-500 transition-all cursor-pointer transform hover:-translate-y-1 relative group"
                    >
                        <button
                            onClick={(e) => {
                                e.stopPropagation();
                                if (!confirm("Stop this client?")) return;
                                stopClient(client.ID).then(fetchClients).catch(console.error);
                            }}
                            className="absolute top-2 right-2 bg-red-600 hover:bg-red-500 text-white p-1 rounded z-10"
                            title="Stop Client"
                        >
                            <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" viewBox="0 0 20 20" fill="currentColor">
                                <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                            </svg>
                        </button>
                        <div className="flex justify-between items-start mb-2">
                            <span className={`px-2 py-0.5 rounded text-xs font-bold font-mono uppercase tracking-wider ${(client.Active) ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}`}>
                                {client.Active ? 'ONLINE' : 'OFFLINE'}
                            </span>
                            <span className="text-xs text-gray-500 font-mono">{client.ID}</span>
                        </div>
                        <div className="space-y-1">
                            <div className="text-sm font-mono text-gray-300 truncate">
                                <span className="text-gray-500">Serial:</span> {client.Serial}
                            </div>
                            <div className="text-sm font-mono text-gray-300 truncate" title={client.Matricule}>
                                <span className="text-gray-500">Auth:</span> {client.Matricule.slice(0, 10)}...
                            </div>
                            <div className="text-xs text-gray-500 mt-2 truncate">
                                Target: {client.ServerURL}
                            </div>
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
};

export default ClientList;
