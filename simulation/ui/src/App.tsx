import React, { useState } from 'react';
import ClientList from './components/ClientList';
import ClientDetail from './components/ClientDetail';
import ScenarioManager from './components/ScenarioManager';

const App: React.FC = () => {
  const [view, setView] = useState<'list' | 'detail' | 'scenarios'>('list');
  const [selectedClientId, setSelectedClientId] = useState<string | null>(null);

  return (
    <div className="bg-gray-900 min-h-screen text-gray-100">
      {/* Header / Nav */}
      <div className="bg-gray-800 border-b border-gray-700 p-4 flex items-center justify-between">
        <h1 className="text-xl font-bold text-white tracking-widest">ESSENSYS SIMULATOR</h1>
        <div className="flex gap-4">
          <button
            onClick={() => { setView('list'); setSelectedClientId(null); }}
            className={`px-4 py-2 rounded font-bold ${view === 'list' || view === 'detail' ? 'bg-blue-600 text-white' : 'text-gray-400 hover:text-white'}`}
          >
            Fleet Control
          </button>
          <button
            onClick={() => { setView('scenarios'); setSelectedClientId(null); }}
            className={`px-4 py-2 rounded font-bold ${view === 'scenarios' ? 'bg-purple-600 text-white' : 'text-gray-400 hover:text-white'}`}
          >
            Scenario Manager
          </button>
        </div>
      </div>

      <div className="container mx-auto mt-4">
        {view === 'list' && (
          <ClientList onSelectClient={(id) => {
            setSelectedClientId(id);
            setView('detail');
          }} />
        )}

        {view === 'detail' && selectedClientId && (
          <ClientDetail
            id={selectedClientId}
            onBack={() => {
              setSelectedClientId(null);
              setView('list');
            }}
          />
        )}

        {view === 'scenarios' && (
          <ScenarioManager />
        )}
      </div>
    </div>
  );
};

export default App;
