import axios from 'axios';

const API_BASE_URL = 'http://localhost:5375/api'; // Simulator Backend Port

export interface Emulator {
    ID: string;
    Serial: string;
    Matricule: string;
    ServerURL: string;
    Active: boolean;
    Values: Record<string, string>;
    History: string[];
}

export interface ScenarioJob {
    index: number;
    value: string;
}

export interface ScenarioStep {
    jobs: ScenarioJob[];
    delay: number;
}

export const getClients = async (): Promise<Emulator[]> => {
    const response = await axios.get(`${API_BASE_URL}/clients`);
    return response.data || [];
};

export const getClient = async (id: string): Promise<Emulator> => {
    const response = await axios.get(`${API_BASE_URL}/clients/${id}`);
    return response.data;
};

export const startSimulation = async (count: number, serverIP: string = 'localhost', serverPort: string = '80', startupScenario: string = '') => {
    let url = `${API_BASE_URL}/simulation/start?count=${count}&serverIP=${serverIP}&serverPort=${serverPort}`;
    if (startupScenario) {
        url += `&startupScenario=${encodeURIComponent(startupScenario)}`;
    }
    await axios.post(url);
};

export const stopSimulation = async () => {
    await axios.post(`${API_BASE_URL}/simulation/stop`);
};

export const stopClient = async (id: string) => {
    await axios.delete(`${API_BASE_URL}/clients/${id}`);
};

// useMock=true calls local scenario, useMock=false calls server injection
export const executeScenario = async (id: string, steps: ScenarioStep[], useMock: boolean = false) => {
    const endpoint = useMock ? 'scenario' : 'inject-scenario';
    await axios.post(`${API_BASE_URL}/clients/${id}/${endpoint}`, steps);
};

export const saveScenario = async (name: string, steps: ScenarioStep[]) => {
    await axios.post(`${API_BASE_URL}/scenarios`, { name, steps });
};

export const getScenarios = async (): Promise<string[]> => {
    const response = await axios.get(`${API_BASE_URL}/scenarios`);
    return response.data;
};

export const loadScenario = async (name: string): Promise<{ name: string, steps: ScenarioStep[] }> => {
    const response = await axios.get(`${API_BASE_URL}/scenarios/${name}`);
    return response.data;
};
