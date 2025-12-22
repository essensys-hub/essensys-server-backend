import React, { useState, useEffect } from 'react';
import { saveScenario, loadScenario, getScenarios } from '../api';
import type { ScenarioStep, ScenarioJob } from '../api';

// Duplicate definition if not exported, better to Refactor later.
// For speed, I'll redefine locally or assume I can export from ClientDetail.
// Actually, I'll move INDEX_CATEGORIES to api or a consts file in next step. For now copy-paste to ensure it works.

const CATEGORIES: { name: string; items: IndexDef[] }[] = [
    {
        name: "Chauffage",
        items: [
            { id: 349, label: "Zone Jour - Auto (Planning)", defaultVal: "1" },
            { id: 349, label: "Zone Jour - Confort", defaultVal: "17" },
            { id: 349, label: "Zone Jour - Eco", defaultVal: "18" },
            { id: 349, label: "Zone Jour - Hors Gel", defaultVal: "21" },
            { id: 349, label: "Zone Jour - OFF", defaultVal: "16" },
            { id: 350, label: "Zone Nuit - Auto", defaultVal: "1" },
            { id: 350, label: "Zone Nuit - Confort", defaultVal: "17" },
            { id: 350, label: "Zone Nuit - OFF", defaultVal: "16" },
            { id: 351, label: "SDB 1 - Auto", defaultVal: "1" },
            { id: 351, label: "SDB 1 - Confort", defaultVal: "17" },
            { id: 351, label: "SDB 1 - OFF", defaultVal: "16" },
            { id: 352, label: "SDB 2 - Auto", defaultVal: "1" },
            { id: 352, label: "SDB 2 - Confort", defaultVal: "17" },
            { id: 352, label: "SDB 2 - OFF", defaultVal: "16" },
            { id: 353, label: "Cumulus - Auto (HC/HP)", defaultVal: "1" },
            { id: 353, label: "Cumulus - OFF", defaultVal: "2" },
        ]
    },
    {
        name: "Eclairage - Principaux",
        items: [
            { id: 616, label: "Terrasse - ON", defaultVal: "4" },
            { id: 610, label: "Terrasse - OFF", defaultVal: "4" },
            { id: 611, label: "Entrée - ON", defaultVal: "1" },
            { id: 605, label: "Entrée - OFF", defaultVal: "1" },
            { id: 613, label: "Escalier - ON", defaultVal: "1" },
            { id: 607, label: "Escalier - OFF", defaultVal: "1" },
            { id: 612, label: "Salon - ON", defaultVal: "128" },
            { id: 606, label: "Salon - OFF", defaultVal: "128" },
            { id: 615, label: "Cuisine - ON", defaultVal: "1" },
            { id: 609, label: "Cuisine - OFF", defaultVal: "1" },
            { id: 616, label: "SDB 1 - ON", defaultVal: "128" },
            { id: 610, label: "SDB 1 - OFF", defaultVal: "128" },
            { id: 615, label: "SDB 2 - ON", defaultVal: "8" },
            { id: 609, label: "SDB 2 - OFF", defaultVal: "8" },
            { id: 614, label: "Gde Chambre - ON", defaultVal: "128" },
            { id: 608, label: "Gde Chambre - OFF", defaultVal: "128" },
            { id: 611, label: "Dressing - ON", defaultVal: "8" },
            { id: 605, label: "Dressing - OFF", defaultVal: "8" },
        ]
    },
    {
        name: "Eclairage - Indirects",
        items: [
            { id: 611, label: "Salon Ind 1 - ON", defaultVal: "2" },
            { id: 605, label: "Salon Ind 1 - OFF", defaultVal: "2" },
            { id: 615, label: "Cuisine Plan - ON", defaultVal: "2" },
            { id: 609, label: "Cuisine Plan - OFF", defaultVal: "2" },
            { id: 613, label: "Chevet Gde Ch 1 - ON", defaultVal: "2" },
            { id: 607, label: "Chevet Gde Ch 1 - OFF", defaultVal: "2" },
        ]
    },
    {
        name: "Volets Roulants",
        items: [
            { id: 617, label: "Volet Salon 1 - Ouvrir", defaultVal: "1" },
            { id: 620, label: "Volet Salon 1 - Fermer", defaultVal: "1" },
            { id: 617, label: "Volet Salon 2 - Ouvrir", defaultVal: "2" },
            { id: 620, label: "Volet Salon 2 - Fermer", defaultVal: "2" },
            { id: 617, label: "Volet SAM 1 - Ouvrir", defaultVal: "8" },
            { id: 620, label: "Volet SAM 1 - Fermer", defaultVal: "8" },
            { id: 619, label: "Volet Cuisine 1 - Ouvrir", defaultVal: "1" },
            { id: 622, label: "Volet Cuisine 1 - Fermer", defaultVal: "1" },
            { id: 618, label: "Volet Gde Ch 1 - Ouvrir", defaultVal: "1" },
            { id: 621, label: "Volet Gde Ch 1 - Fermer", defaultVal: "1" },
            { id: 617, label: "Volet Bureau - Ouvrir", defaultVal: "32" },
            { id: 620, label: "Volet Bureau - Fermer", defaultVal: "32" },
            { id: 619, label: "Store - Ouvrir", defaultVal: "8" },
            { id: 622, label: "Store - Fermer", defaultVal: "8" },
        ]
    },
    {
        name: "Divers",
        items: [
            { id: 363, label: "Arrosage - Auto", defaultVal: "255" },
            { id: 363, label: "Arrosage - 15min", defaultVal: "15" },
            { id: 363, label: "Arrosage - OFF", defaultVal: "0" },
            { id: 440, label: "Prise Sécu - ON", defaultVal: "0" },
            { id: 440, label: "Prise Sécu - OFF", defaultVal: "1" },
            { id: 590, label: "Lancer Scénario", defaultVal: "1", hint: "Value = Scenario ID" }
        ]
    }
];

interface IndexDef {
    id: number;
    label: string;
    defaultVal: string;
    hint?: string;
}

const ScenarioManager: React.FC = () => {
    // State
    const [scenarioSteps, setScenarioSteps] = useState<ScenarioStep[]>([]);
    const [scenarioName, setScenarioName] = useState("");
    const [savedScenarios, setSavedScenarios] = useState<string[]>([]);

    const [currentDelay, setCurrentDelay] = useState(2000);
    const [currentJobs, setCurrentJobs] = useState<ScenarioJob[]>([]);
    const [selectedIdx, setSelectedIdx] = useState(613);
    const [customIdx, setCustomIdx] = useState("");
    const [jobValue, setJobValue] = useState("1");

    useEffect(() => {
        refreshList();
    }, []);

    const refreshList = () => {
        getScenarios().then(data => setSavedScenarios(data || [])).catch(console.error);
    };

    const load = async (name: string) => {
        if (!name) return;
        try {
            const data = await loadScenario(name);
            setScenarioName(data.name);
            setScenarioSteps(data.steps || []);
        } catch (e) {
            alert("Error loading scenario");
        }
    };

    const save = async () => {
        if (!scenarioName) return alert("Enter a name");
        try {
            await saveScenario(scenarioName, scenarioSteps);
            alert("Saved!");
            refreshList();
        } catch (e) {
            alert("Error saving");
        }
    };

    // Helper to find def
    const getScaleDef = (idx: number) => {
        for (const cat of CATEGORIES) {
            const found = cat.items.find(i => i.id === idx);
            if (found) return found;
        }
        return null;
    };

    const handleIndexChange = (newIdx: number) => {
        setSelectedIdx(newIdx);
        const def = getScaleDef(newIdx);
        if (def && def.defaultVal) {
            setJobValue(def.defaultVal);
        }
    };

    const addJobToStep = () => {
        const idx = (selectedIdx === -1) ? Number(customIdx) : selectedIdx;
        setCurrentJobs([...currentJobs, { index: idx, value: jobValue }]);
    };

    const removeJob = (idxToRemove: number) => {
        setCurrentJobs(currentJobs.filter((_, i) => i !== idxToRemove));
    };

    const addStepToScenario = () => {
        if (currentJobs.length === 0) return;
        setScenarioSteps([...scenarioSteps, { jobs: [...currentJobs], delay: currentDelay }]);
        setCurrentJobs([]);
    };

    return (
        <div className="p-4 space-y-6">
            <div className="bg-gray-800 p-6 rounded-lg border border-gray-700">
                <h2 className="text-2xl font-bold text-white mb-4">Scenario Manager</h2>

                {/* Header Controls */}
                <div className="flex gap-4 mb-6 bg-gray-900 p-4 rounded items-center">
                    <div className="flex-1">
                        <label className="text-xs text-gray-400 block mb-1">Current Scenario Name</label>
                        <input
                            value={scenarioName}
                            onChange={e => setScenarioName(e.target.value)}
                            className="w-full p-2 bg-gray-700 text-white border border-gray-600 rounded"
                            placeholder="My_Test_Scenario"
                        />
                    </div>
                    <div>
                        <label className="text-xs text-gray-400 block mb-1">Actions</label>
                        <div className="flex gap-2">
                            <button onClick={save} className="bg-purple-600 hover:bg-purple-500 text-white px-4 py-2 rounded font-bold">Save</button>
                            <button onClick={() => { setScenarioSteps([]); setScenarioName(""); }} className="bg-gray-600 hover:bg-gray-500 text-white px-4 py-2 rounded">New / Clear</button>
                        </div>
                    </div>
                    <div className="flex-1">
                        <label className="text-xs text-gray-400 block mb-1">Load Existing</label>
                        <select onChange={e => load(e.target.value)} className="w-full p-2 bg-gray-700 text-white border border-gray-600 rounded">
                            <option value="">Select a scenario...</option>
                            {savedScenarios?.map(s => <option key={s} value={s}>{s}</option>)}
                        </select>
                    </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                    {/* BUILDER (Left) */}
                    <div className="space-y-4">
                        <div className="bg-gray-700 p-4 rounded border border-gray-600">
                            <h4 className="text-white font-bold mb-2">Editor</h4>

                            {/* Job Builder */}
                            <div className="flex flex-col gap-2 mb-4">
                                <label className="text-xs text-gray-400">Add Action (Job)</label>
                                <div className="flex flex-wrap gap-2">
                                    <select
                                        value={selectedIdx}
                                        onChange={e => handleIndexChange(Number(e.target.value))}
                                        className="p-2 rounded bg-gray-600 text-white border border-gray-500 flex-1 text-sm min-w-[200px]"
                                    >
                                        {CATEGORIES.map(cat => (
                                            <optgroup key={cat.name} label={cat.name}>
                                                {cat.items.map((item, idx) => (
                                                    <option key={`${item.id}-${idx}`} value={item.id}>{item.id} - {item.label}</option>
                                                ))}
                                            </optgroup>
                                        ))}
                                        <optgroup label="Manuel">
                                            <option value={-1}>Index personnalisé...</option>
                                        </optgroup>
                                    </select>
                                    {selectedIdx === -1 && (
                                        <input type="number" placeholder="Idx" value={customIdx} onChange={e => setCustomIdx(e.target.value)} className="p-2 rounded bg-gray-600 text-white w-16" />
                                    )}
                                    <input type="text" placeholder="Val" value={jobValue} onChange={e => setJobValue(e.target.value)} className="p-2 rounded bg-gray-600 text-white w-24" />
                                    <button onClick={addJobToStep} className="bg-blue-600 text-white px-3 rounded font-bold">+</button>
                                </div>
                                {getScaleDef(selectedIdx)?.hint && <div className="text-blue-300 text-xs italic">{getScaleDef(selectedIdx)?.hint}</div>}

                                {currentJobs.length > 0 && (
                                    <div className="bg-gray-900 p-2 rounded mt-2">
                                        <div className="text-xs text-gray-500 mb-1">Jobs in current step:</div>
                                        {currentJobs.map((job, i) => (
                                            <div key={i} className="flex justify-between items-center bg-gray-800 p-1 rounded mb-1">
                                                <div className="text-xs text-gray-300 font-mono pl-2">- [{job.index}] = "{job.value}"</div>
                                                <button
                                                    onClick={() => removeJob(i)}
                                                    className="text-red-500 hover:text-red-400 px-2 font-bold text-xs"
                                                >
                                                    X
                                                </button>
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>

                            <div className="flex flex-col gap-2 mb-4">
                                <label className="text-xs text-gray-400">Step Delay (ms)</label>
                                <input type="number" value={currentDelay} onChange={e => setCurrentDelay(Number(e.target.value))} className="p-2 rounded bg-gray-600 text-white w-32" />
                            </div>

                            <button onClick={addStepToScenario} disabled={currentJobs.length === 0} className="w-full bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white py-2 rounded font-bold">Add Step &darr;</button>
                        </div>
                    </div>

                    {/* PREVIEW (Right) */}
                    <div className="space-y-4">
                        <div className="bg-black p-4 rounded border border-gray-600 h-full flex flex-col">
                            <h4 className="text-gray-400 text-sm mb-2 font-bold">Preview</h4>
                            <div className="flex-1 overflow-y-auto space-y-2 mb-4 max-h-[500px]">
                                {scenarioSteps.length === 0 && <div className="text-gray-600 text-sm italic text-center mt-10">Empty Scenario</div>}
                                {scenarioSteps.map((step, idx) => (
                                    <div key={idx} className="bg-gray-900 border border-gray-700 p-2 rounded relative group">
                                        <div className="flex justify-between items-start">
                                            <span className="text-xs font-bold text-indigo-400">Step {idx + 1}</span>
                                            <span className="text-xs text-gray-500 font-mono">Wait {step.delay}ms</span>
                                        </div>
                                        <div className="mt-1 space-y-0.5 pl-2 border-l-2 border-gray-700">
                                            {step.jobs.map((job, j) => (
                                                <div key={j} className="text-xs text-gray-300 font-mono">[{job.index}] &larr; <span className="text-yellow-400">{job.value}</span></div>
                                            ))}
                                        </div>
                                        <button
                                            onClick={() => {
                                                const newSteps = [...scenarioSteps];
                                                newSteps.splice(idx, 1);
                                                setScenarioSteps(newSteps);
                                            }}
                                            className="absolute top-2 right-2 text-red-500 hover:text-red-400 opacity-0 group-hover:opacity-100 text-xs font-bold"
                                        >
                                            X
                                        </button>
                                    </div>
                                ))}
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default ScenarioManager;
