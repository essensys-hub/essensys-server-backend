
import React, { useEffect, useState } from 'react';
import { getClient } from '../api';
import type { Emulator } from '../api';

interface ClientDetailProps {
    id: string;
    onBack: () => void;
}



// Categories removed as builder was removed.

const ClientDetail: React.FC<ClientDetailProps> = ({ id, onBack }) => {
    const [client, setClient] = useState<Emulator | null>(null);

    // Scenario Builder State Removed
    // const [scenarioSteps, setScenarioSteps] = useState<ScenarioStep[]>([]);

    const fetchClient = async () => {
        try {
            const data = await getClient(id);
            setClient(data);
        } catch (e: any) {
            console.error("Failed to fetch client", e);
            // Alert only once or show in UI? Alert might span.
            // Let's use a temporary alert to confirm connection rejection.
            alert("Debug: Failed to fetch client: " + e.message);
        }
    };

    // Clean up unused functions from removed builder
    // addJobToStep, addStepToScenario, runScenario removed

    useEffect(() => {
        fetchClient();
        const interval = setInterval(fetchClient, 1000); // 1s refresh for details
        return () => clearInterval(interval);
    }, [id]);

    if (!client) {
        return <div className="text-white p-4">Loading...</div>;
    }

    return (
        <div className="p-4 space-y-6">
            <button
                onClick={onBack}
                className="bg-gray-700 hover:bg-gray-600 text-white px-4 py-2 rounded mb-4"
            >
                &larr; Back to Fleet
            </button>

            <div className="bg-gray-800 p-6 rounded-lg border border-gray-700">
                <div className="flex justify-between items-center mb-4">
                    <h2 className="text-2xl font-bold text-white">Client: <span className="text-blue-400">{client.ID}</span></h2>
                    <span className={`px-3 py-1 rounded text-sm font-mono ${(client.Active) ? 'bg-green-900 text-green-300' : 'bg-red-900 text-red-300'}`}>
                        {client.Active ? 'ONLINE' : 'OFFLINE'}
                    </span>
                </div>
                <div className="grid grid-cols-2 gap-4 text-sm text-gray-300">
                    <div><span className="text-gray-500">Serial:</span> {client.Serial}</div>
                    <div><span className="text-gray-500">Matricule:</span> {client.Matricule}</div>
                </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
                {/* Reference Table View */}
                <div className="bg-gray-800 p-6 rounded-lg border border-gray-700">
                    <h3 className="text-xl font-bold text-white mb-4">Reference Table (Indices 600-639)</h3>
                    <div className="overflow-x-auto max-h-96 overflow-y-auto">
                        <table className="w-full text-sm text-left text-gray-300">
                            <thead className="text-xs text-gray-400 uppercase bg-gray-700 sticky top-0">
                                <tr>
                                    <th className="px-4 py-2">Index</th>
                                    <th className="px-4 py-2">Value</th>
                                </tr>
                            </thead>
                            <tbody>
                                {Object.entries(client.Values || {}).sort((a, b) => Number(a[0]) - Number(b[0])).map(([k, v]) => (
                                    <tr key={k} className="border-b border-gray-700 hover:bg-gray-750">
                                        <td className="px-4 py-2 font-mono text-blue-400">{k}</td>
                                        <td className="px-4 py-2 font-mono">{v}</td>
                                    </tr>
                                ))}
                                {(!client.Values || Object.keys(client.Values).length === 0) && (
                                    <tr>
                                        <td colSpan={2} className="px-4 py-2 text-center text-gray-500">No data</td>
                                    </tr>
                                )}
                            </tbody>
                        </table>
                    </div>
                </div>

                {/* History View */}
                <div className="bg-gray-800 p-6 rounded-lg border border-gray-700">
                    <h3 className="text-xl font-bold text-white mb-4">Last 20 Events</h3>
                    <div className="bg-black rounded p-4 h-96 overflow-y-auto font-mono text-xs text-green-400">
                        {client.History && client.History.slice().reverse().map((entry, i) => (
                            <div key={i} className="mb-1 border-b border-gray-900 pb-1">
                                {entry}
                            </div>
                        ))}
                        {(!client.History || client.History.length === 0) && (
                            <div className="text-gray-500 italic">No history yet</div>
                        )}
                    </div>
                </div>
            </div>

            {/* Scenario Builder removed as per request */}
        </div>
    );
};

export default ClientDetail;
