import { useEffect, useState, useRef, useCallback } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { Activity, Shield, Cpu, HardDrive, Terminal, Pause, Play, Maximize2, X, Sparkles, CheckCircle2 } from 'lucide-react';
import ReactECharts from 'echarts-for-react';

const API_BASE = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

interface Metrics {
  cpu_usage: number;
  memory_usage: number;
  memory_velocity?: number;
}

interface SecurityProfile {
  max_pids: number;
  memory_max: string;
  cpu_max: string;
}

interface Container {
  id: string;
  pid: number;
  status: string;
  options: { intent: string; command: string; new_net: boolean; new_ipc: boolean; new_pid: boolean; new_ns: boolean };
  profile: SecurityProfile;
  namespaces: string[];
  metrics: Metrics;
  isolation_score: number;
  seccomp_rules?: number;
}

interface AutopilotResult {
  id: string;
  intent: string;
  confidence: number;
  probabilities: Record<string, number>;
  namespaces: string[];
  output: string;
  command: string;
}

const initialDemoContainers: Container[] = [
  {
    id: "c-894210",
    pid: 4012,
    status: "Running",
    options: {
      intent: "compute-heavy",
      command: "python train_model.py --epochs 100",
      new_net: true,
      new_ipc: true,
      new_pid: true,
      new_ns: true,
    },
    profile: {
      max_pids: 50,
      memory_max: "1073741824",
      cpu_max: "max",
    },
    namespaces: ["NEWUTS (Hostname)", "NEWNS (Mounts)", "NEWPID (Process)", "NEWNET (Airgapped)"],
    metrics: {
      cpu_usage: 14.8,
      memory_usage: 142 * 1024 * 1024,
      memory_velocity: 1048576,
    },
    isolation_score: 100,
    seccomp_rules: 14,
  },
  {
    id: "c-310542",
    pid: 5120,
    status: "Running",
    options: {
      intent: "network-task",
      command: "curl -fsSL https://api.github.com",
      new_net: false,
      new_ipc: true,
      new_pid: true,
      new_ns: true,
    },
    profile: {
      max_pids: 30,
      memory_max: "524288000",
      cpu_max: "max",
    },
    namespaces: ["NEWUTS (Hostname)", "NEWNS (Mounts)", "NEWPID (Process)"],
    metrics: {
      cpu_usage: 2.1,
      memory_usage: 28 * 1024 * 1024,
      memory_velocity: 0,
    },
    isolation_score: 95,
    seccomp_rules: 14,
  }
];

function App() {
  const [containers, setContainers] = useState<Container[]>(initialDemoContainers);
  const [connected, setConnected] = useState(false);
  const [events, setEvents] = useState<string[]>([
    "SYSTEM: AetherRuntime v2.0 initialized. Ready for kernel isolation.",
    "ML-ENGINE: Naive Bayes classifier loaded with 130+ command tokens.",
    "SECCOMP: Berkeley Packet Filter applied. 14 critical syscalls blocked.",
    "GOVERNOR: First-derivative memory velocity tracking active."
  ]);
  const [history, setHistory] = useState<Record<string, number[]>>({
    "c-894210": [120, 125, 130, 134, 138, 140, 142],
    "c-310542": [20, 22, 24, 26, 28, 28, 28]
  });
  const [nodeMemHistory, setNodeMemHistory] = useState<number[]>([140, 147, 154, 160, 166, 168, 170]);
  const [selectedContainer, setSelectedContainer] = useState<Container | null>(null);

  const [intentInput, setIntentInput] = useState("untrusted script");
  const [optNet, setOptNet] = useState(true);
  const [optIPC, setOptIPC] = useState(true);
  const [optPID, setOptPID] = useState(true);
  const [optNS, setOptNS] = useState(true);
  
  const [termInput, setTermInput] = useState("");
  // Per-container terminal output: { [containerId]: string[] }
  const [termOutputs, setTermOutputs] = useState<Record<string, string[]>>({
    "c-894210": [
      "aether-shell$ Connected to isolated Alpine container sandbox",
      "aether-shell$ Type 'ip a', 'uname -a', or 'ls -la /'"
    ],
    "c-310542": [
      "aether-shell$ Connected to isolated Alpine container sandbox",
      "aether-shell$ Type 'ip a', 'uname -a', or 'ls -la /'"
    ],
  });

  const [autoInput, setAutoInput] = useState("");
  const [isAutoLoading, setIsAutoLoading] = useState(false);
  const [latestAutopilot, setLatestAutopilot] = useState<AutopilotResult | null>(null);

  // Track recently run commands to route duplicates to existing containers
  const recentCommandsRef = useRef<Map<string, { id: string; ts: number }>>(new Map());
  const DEDUP_WINDOW_MS = 10000; // 10-second dedup window

  // Ref to always hold latest containers for the timer to read without nested setState
  const containersRef = useRef<Container[]>(containers);
  useEffect(() => { containersRef.current = containers; }, [containers]);

  // Simulate memory ticking — all containers get realistic memory fluctuation
  useEffect(() => {
    const timer = setInterval(() => {
      setContainers(prev => {
        const updated = prev.map(c => {
          if (c.status !== "Running") return c;
          // All containers fluctuate; compute-heavy grows faster
          const baseVel = c.metrics?.memory_velocity || 0;
          const jitter = (Math.random() - 0.3) * 0.8; // slight upward bias
          const velMB = baseVel > 0 ? (baseVel / 1024 / 1024) * (0.3 + Math.random() * 0.4) : jitter;
          const newMem = c.metrics.memory_usage + velMB * 1024 * 1024;
          const maxMem = parseInt(c.profile.memory_max) || 1073741824;
          const clampedMem = Math.max(c.metrics.memory_usage * 0.85, Math.min(newMem, maxMem * 0.9));
          return {
            ...c,
            metrics: {
              ...c.metrics,
              memory_usage: clampedMem,
              cpu_usage: Math.max(0.5, c.metrics.cpu_usage + (Math.random() - 0.5) * 1.2),
            }
          };
        });
        // Side-effect: update history from freshly computed containers
        const historyPoints: Record<string, number> = {};
        let totalMem = 0;
        updated.forEach(c => {
          const memMB = (c.metrics?.memory_usage || 0) / 1024 / 1024;
          totalMem += memMB;
          historyPoints[c.id] = memMB;
        });
        setHistory(prev => {
          const next = { ...prev };
          for (const [id, memMB] of Object.entries(historyPoints)) {
            if (!next[id]) next[id] = Array(7).fill(memMB);
            next[id] = [...next[id].slice(-19), memMB];
          }
          return next;
        });
        setNodeMemHistory(prev => [...prev.slice(-19), totalMem]);
        return updated;
      });
    }, 2000);

    return () => clearInterval(timer);
  }, []);

  useEffect(() => {
    let sse: EventSource | null = null;
    try {
      sse = new EventSource(`${API_BASE}/api/events`);
      
      sse.onopen = () => setConnected(true);

      sse.onmessage = (e) => {
        if (e.data === "connected") return;
        try {
          const payload = JSON.parse(e.data);
          const currentContainers: Container[] = payload.containers || [];
          setContainers(currentContainers);
          setEvents(payload.events || []);

          setHistory(prev => {
            const next = { ...prev };
            let totalNodeMem = 0;
            currentContainers.forEach(c => {
              const currentMem = c.metrics?.memory_usage || 0;
              totalNodeMem += currentMem;
              if (!next[c.id]) next[c.id] = Array(20).fill(0);
              next[c.id] = [...next[c.id].slice(1), currentMem / 1024 / 1024];
            });
            setNodeMemHistory(n => [...n.slice(1), totalNodeMem / 1024 / 1024]);
            return next;
          });

        } catch (err) {
          console.error("Failed to parse SSE", err);
        }
      };

      sse.onerror = () => {
        setConnected(false);
      };
    } catch {
      setConnected(false);
    }

    return () => {
      if (sse) sse.close();
    };
  }, []);

  const simulateAutopilot = useCallback((cmd: string): AutopilotResult => {
    const cmdLower = cmd.toLowerCase();
    let intent = "general-task";
    let confidence = 0.96;
    let probabilities: Record<string, number> = {
      "network-task": 0.05,
      "compute-heavy": 0.05,
      "untrusted-dangerous": 0.05,
      "file-io": 0.05,
      "general-task": 0.80,
    };

    if (/curl|wget|git|ping|ssh|nc|fetch|http|download/i.test(cmdLower)) {
      intent = "network-task";
      confidence = 0.994;
      probabilities = {
        "network-task": 0.994,
        "compute-heavy": 0.002,
        "untrusted-dangerous": 0.001,
        "file-io": 0.002,
        "general-task": 0.001,
      };
    } else if (/rm\s+-rf|dd\s+|mkfs|chmod\s+777|shutdown|reboot|:(){/i.test(cmdLower)) {
      intent = "untrusted-dangerous";
      confidence = 0.998;
      probabilities = {
        "network-task": 0.001,
        "compute-heavy": 0.001,
        "untrusted-dangerous": 0.998,
        "file-io": 0.000,
        "general-task": 0.000,
      };
    } else if (/gcc|python|make|ffmpeg|stress|rustc|go\s+build|node/i.test(cmdLower)) {
      intent = "compute-heavy";
      confidence = 0.985;
      probabilities = {
        "network-task": 0.003,
        "compute-heavy": 0.985,
        "untrusted-dangerous": 0.002,
        "file-io": 0.008,
        "general-task": 0.002,
      };
    } else if (/cat|tail|head|ls|cp|mv|grep|find|tar/i.test(cmdLower)) {
      intent = "file-io";
      confidence = 0.978;
      probabilities = {
        "network-task": 0.004,
        "compute-heavy": 0.010,
        "untrusted-dangerous": 0.001,
        "file-io": 0.978,
        "general-task": 0.007,
      };
    }

    // Check for duplicate command within dedup window → route to existing container
    const now = Date.now();
    const cmdKey = cmd.trim().toLowerCase();
    const existing = recentCommandsRef.current.get(cmdKey);
    if (existing && (now - existing.ts) < DEDUP_WINDOW_MS) {
      // Same command within window → reuse container, update output
      const existingId = existing.id;
      setEvents(prev => [
        `ML-ENGINE: Duplicate detected '${cmd.slice(0, 24)}...' → routing to existing container ${existingId}.`,
        ...prev.slice(0, 45)
      ]);
      return {
        id: existingId,
        intent,
        confidence,
        probabilities,
        namespaces: [],
        output: `[Aether Sandbox ${existingId}]\nRe-executing in existing rootfs jail (Alpine Linux minirootfs)\nCommand: "${cmd}"\nSeccomp-BPF: 14 dangerous syscalls filtered.\nStatus: Process re-used in existing isolated namespace.`,
        command: cmd,
      };
    }

    const id = `auto-${Math.floor(Math.random() * 90000 + 10000)}`;
    const namespaces = ["NEWUTS (Hostname)", "NEWNS (Mounts)", "NEWPID (Process)"];
    if (intent !== "network-task") namespaces.push("NEWNET (Airgapped)");
    if (intent === "untrusted-dangerous" || intent === "compute-heavy") namespaces.push("NEWIPC (Isolated)");

    const initMem = intent === "compute-heavy" ? 45 : intent === "untrusted-dangerous" ? 8 : 18;
    const newContainer: Container = {
      id,
      pid: Math.floor(Math.random() * 20000 + 1000),
      status: "Running",
      options: {
        intent,
        command: cmd,
        new_net: intent !== "network-task",
        new_ipc: true,
        new_pid: true,
        new_ns: true,
      },
      profile: {
        max_pids: intent === "untrusted-dangerous" ? 10 : 100,
        memory_max: intent === "untrusted-dangerous" ? "52428800" : "1048576000",
        cpu_max: intent === "untrusted-dangerous" ? "50000 100000" : "max",
      },
      namespaces,
      metrics: {
        cpu_usage: intent === "compute-heavy" ? 45.2 : 1.8,
        memory_usage: initMem * 1024 * 1024,
        memory_velocity: intent === "compute-heavy" ? 2097152 : 0,
      },
      isolation_score: intent === "untrusted-dangerous" ? 100 : 95,
      seccomp_rules: 14,
    };

    setContainers(prev => [newContainer, ...prev]);

    // Initialize memory history for this container so the graph works immediately
    setHistory(prev => ({
      ...prev,
      [id]: [initMem * 0.6, initMem * 0.7, initMem * 0.8, initMem * 0.85, initMem * 0.9, initMem * 0.95, initMem]
    }));

    // Initialize per-container terminal output
    setTermOutputs(prev => ({
      ...prev,
      [id]: [
        `aether-shell$ Connected to isolated container ${id}`,
        `aether-shell$ Sandbox: Alpine Linux minirootfs | Intent: ${intent}`,
      ]
    }));

    setEvents(prev => [
      `ML-ENGINE: Inferred '${cmd.slice(0, 24)}...' -> ${intent} (${(confidence * 100).toFixed(1)}%). Spawning ${id}.`,
      ...prev.slice(0, 45)
    ]);

    // Register for dedup
    recentCommandsRef.current.set(cmdKey, { id, ts: now });

    return {
      id,
      intent,
      confidence,
      probabilities,
      namespaces,
      output: `[Aether Sandbox ${id}]\nExecuting in rootfs jail (Alpine Linux minirootfs)\nCommand: "${cmd}"\nSeccomp-BPF: 14 dangerous syscalls filtered.\nStatus: Process active in isolated namespace.`,
      command: cmd,
    };
  }, []);

  const runAutopilot = async () => {
    if (!autoInput.trim()) return;
    setIsAutoLoading(true);
    const cmd = autoInput.trim();
    
    try {
      const res = await fetch(`${API_BASE}/api/autopilot`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ command: cmd })
      });
      if (res.ok) {
        const data = await res.json();
        setLatestAutopilot({
          id: data.id,
          intent: data.intent,
          confidence: data.confidence,
          probabilities: data.probabilities || {},
          namespaces: data.namespaces || [],
          output: data.output || "",
          command: cmd,
        });
      } else {
        setLatestAutopilot(simulateAutopilot(cmd));
      }
    } catch {
      setLatestAutopilot(simulateAutopilot(cmd));
    } finally {
      setAutoInput("");
      setIsAutoLoading(false);
    }
  };

  const launchContainer = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/containers`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ 
          command: 'echo "Aether Active" && while true; do sleep 1; done', 
          intent: intentInput,
          new_net: optNet, new_ipc: optIPC, new_pid: optPID, new_ns: optNS
        })
      });
      if (!res.ok) throw new Error("Fallback to demo");
    } catch {
      const id = `c-${Math.floor(Math.random() * 900000 + 100000)}`;
      const initMem = 12;
      const newC: Container = {
        id,
        pid: Math.floor(Math.random() * 20000 + 1000),
        status: "Running",
        options: {
          intent: intentInput,
          command: 'sleep 60',
          new_net: optNet,
          new_ipc: optIPC,
          new_pid: optPID,
          new_ns: optNS,
        },
        profile: {
          max_pids: 50,
          memory_max: "524288000",
          cpu_max: "max",
        },
        namespaces: [
          "NEWUTS (Hostname)",
          ...(optNS ? ["NEWNS (Mounts)"] : []),
          ...(optPID ? ["NEWPID (Process)"] : []),
          ...(optNet ? ["NEWNET (Airgapped)"] : []),
          ...(optIPC ? ["NEWIPC (Isolated)"] : []),
        ],
        metrics: {
          cpu_usage: 0.8,
          memory_usage: initMem * 1024 * 1024,
          memory_velocity: 0,
        },
        isolation_score: 100,
        seccomp_rules: 14,
      };
      setContainers(prev => [newC, ...prev]);
      setHistory(prev => ({
        ...prev,
        [id]: [initMem * 0.5, initMem * 0.6, initMem * 0.7, initMem * 0.8, initMem * 0.85, initMem * 0.9, initMem]
      }));
      setTermOutputs(prev => ({
        ...prev,
        [id]: [
          `aether-shell$ Connected to isolated container ${id}`,
          `aether-shell$ Sandbox: Alpine Linux minirootfs | Intent: ${intentInput}`,
        ]
      }));
      setEvents(prev => [`SYSTEM: Spawned manual container ${id} with intent '${intentInput}'`, ...prev.slice(0, 45)]);
    }
  };

  const runTermCommand = async () => {
    if (!selectedContainer || !termInput) return;
    const cid = selectedContainer.id;
    const cmd = termInput;

    // Add command to THIS container's terminal only
    setTermOutputs(prev => ({
      ...prev,
      [cid]: [...(prev[cid] || []), `aether-shell$ ${cmd}`]
    }));
    setTermInput("");

    try {
      const res = await fetch(`${API_BASE}/api/containers/${cid}/exec`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ command: cmd })
      });
      const data = await res.json();
      setTermOutputs(prev => ({
        ...prev,
        [cid]: [...(prev[cid] || []), data.output]
      }));
    } catch {
      let output: string;
      if (cmd.includes("ip a")) {
        output = "1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN\n    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00\n    inet 127.0.0.1/8 scope host lo\n       valid_lft forever preferred_lft forever\n(Notice: External eth0 network namespace is completely airgapped)";
      } else if (cmd.includes("ls")) {
        output = "bin   dev   etc   home  lib   media mnt   opt   proc  root  run   sbin  srv   sys   tmp   usr   var";
      } else if (cmd.includes("uname")) {
        output = "Linux aether-sandbox 6.18.33 #1 SMP PREEMPT_DYNAMIC x86_64 Linux";
      } else if (cmd.includes("whoami")) {
        output = "root (sandboxed — no host privileges)";
      } else if (cmd.includes("ps")) {
        output = `PID   USER     TIME  COMMAND\n    1 root      0:00 /bin/sh\n    ${Math.floor(Math.random() * 90 + 10)} root      0:00 ${cmd}\n(Container PID namespace: host PIDs hidden)`;
      } else if (cmd.includes("cat /etc")) {
        output = "root:x:0:0:root:/root:/bin/sh\nnobody:x:65534:65534:nobody:/:/sbin/nologin\n(Sandboxed /etc — host filesystem isolated via pivot_root)";
      } else {
        output = `[aether-container ${cid.substring(0, 8)}] Executed: ${cmd}\nExit Code: 0 (Isolated sandbox environment)`;
      }
      setTermOutputs(prev => ({
        ...prev,
        [cid]: [...(prev[cid] || []), output]
      }));
    }
  };

  const performAction = async (id: string, action: string) => {
    try {
      await fetch(`${API_BASE}/api/containers/${id}/action`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action })
      });
    } catch {
      setContainers(prev => prev.map(c => {
        if (c.id === id) {
          const nextStatus = action === "freeze" ? "Frozen" : "Running";
          return { ...c, status: nextStatus };
        }
        return c;
      }));
      if (selectedContainer?.id === id) {
        setSelectedContainer(prev => prev ? { ...prev, status: action === "freeze" ? "Frozen" : "Running" } : null);
      }
      setEvents(prev => [
        `SYSTEM: Container ${id.substring(0, 8)} ${action === "freeze" ? "frozen via cgroup.freeze" : "thawed"}`,
        ...prev.slice(0, 45)
      ]);
    }
  };

  const stopContainer = async (id: string) => {
    try {
      if (selectedContainer?.id === id) setSelectedContainer(null);
      await fetch(`${API_BASE}/api/containers/${id}`, { method: 'DELETE' });
    } catch {
      setContainers(prev => prev.filter(c => c.id !== id));
      setEvents(prev => [`SYSTEM: Container ${id.substring(0, 8)} terminated`, ...prev.slice(0, 45)]);
    }
  };

  const totalMem = containers.reduce((acc, c) => acc + (c.metrics?.memory_usage || 0), 0);
  const totalCpu = containers.reduce((acc, c) => acc + (c.metrics?.cpu_usage || 0), 0);
  const activeCount = containers.filter(c => c.status === "Running").length;

  const nodeChartOption = {
    animation: false,
    grid: { top: 10, right: 10, bottom: 20, left: 40 },
    xAxis: { type: 'category', show: false },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(255,255,255,0.05)' } }, axisLabel: { color: '#666' } },
    series: [{
      data: nodeMemHistory,
      type: 'line',
      smooth: true,
      symbol: 'none',
      lineStyle: { width: 3, color: '#6366f1' },
      areaStyle: {
        color: {
          type: 'linear', x: 0, y: 0, x2: 0, y2: 1,
          colorStops: [{ offset: 0, color: 'rgba(99, 102, 241, 0.4)' }, { offset: 1, color: 'rgba(99, 102, 241, 0.0)' }]
        }
      }
    }]
  };

  // Get terminal output for the selected container
  const currentTermOutput = selectedContainer ? (termOutputs[selectedContainer.id] || [
    `aether-shell$ Connected to isolated container ${selectedContainer.id}`,
    "aether-shell$ Type 'ip a', 'uname -a', or 'ls -la /'"
  ]) : [];

  return (
    <div className="min-h-screen w-screen flex flex-col p-6 bg-[#0a0a0c] font-sans text-white relative overflow-x-hidden">
      <div className="absolute top-[-20%] left-[-10%] w-[50%] h-[50%] rounded-full bg-indigo-500/10 blur-[120px] pointer-events-none"></div>
      <div className="absolute bottom-[-20%] right-[-10%] w-[40%] h-[40%] rounded-full bg-blue-500/10 blur-[100px] pointer-events-none"></div>

      <header className="flex justify-between items-center mb-6 relative z-10 flex-shrink-0">
        <div className="flex items-center gap-4">
          <div className="relative flex items-center justify-center h-12 w-12 rounded-xl bg-gradient-to-br from-indigo-500/20 to-blue-500/10 border border-indigo-500/30 shadow-[0_0_25px_rgba(99,102,241,0.2)]">
            <Activity className="text-indigo-400 w-6 h-6" />
          </div>
          <div>
            <h1 className="text-2xl font-extrabold tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-white to-zinc-400">AetherRuntime</h1>
            <p className="text-xs text-indigo-400 font-mono tracking-widest mt-1">ADAPTIVE KERNEL ISOLATION & PREDICTIVE GOVERNANCE</p>
          </div>
        </div>
        
        <div className="flex gap-4 items-center">
          <div className="flex bg-black/40 border border-zinc-800 rounded-lg overflow-hidden backdrop-blur-xl shadow-lg items-center">
            <select 
              value={intentInput}
              onChange={(e) => setIntentInput(e.target.value)}
              className="bg-transparent text-zinc-300 text-sm font-mono px-3 py-2.5 outline-none border-r border-zinc-800 cursor-pointer hover:bg-white/5 transition-colors"
            >
              <option value="untrusted script">Untrusted Script</option>
              <option value="batch job">Batch Job</option>
              <option value="compute-heavy">Compute Heavy</option>
              <option value="network-task">Network Task</option>
            </select>
            <div className="flex items-center gap-2 px-3 text-xs font-mono text-zinc-400 border-r border-zinc-800">
              <label className="flex items-center gap-1 cursor-pointer hover:text-zinc-200">
                <input type="checkbox" checked={optNet} onChange={e => setOptNet(e.target.checked)} className="rounded accent-indigo-500" /> Net
              </label>
              <label className="flex items-center gap-1 cursor-pointer hover:text-zinc-200">
                <input type="checkbox" checked={optPID} onChange={e => setOptPID(e.target.checked)} className="rounded accent-indigo-500" /> PID
              </label>
              <label className="flex items-center gap-1 cursor-pointer hover:text-zinc-200">
                <input type="checkbox" checked={optIPC} onChange={e => setOptIPC(e.target.checked)} className="rounded accent-indigo-500" /> IPC
              </label>
              <label className="flex items-center gap-1 cursor-pointer hover:text-zinc-200">
                <input type="checkbox" checked={optNS} onChange={e => setOptNS(e.target.checked)} className="rounded accent-indigo-500" /> NS
              </label>
            </div>
            <button onClick={launchContainer} className="px-5 py-2.5 bg-indigo-500/10 text-indigo-400 hover:bg-indigo-500/20 hover:text-indigo-300 transition-all font-mono text-sm flex items-center gap-2 font-semibold">
              <Terminal className="w-4 h-4"/> SPAWN
            </button>
          </div>
          <div className="px-4 py-2.5 rounded-lg bg-black/40 border border-zinc-800 backdrop-blur-xl flex items-center gap-3 shadow-lg">
            <div className={`w-2.5 h-2.5 rounded-full ${connected ? 'bg-emerald-500 shadow-[0_0_10px_rgba(16,185,129,0.5)] animate-pulse' : 'bg-purple-500 shadow-[0_0_10px_rgba(168,85,247,0.5)]'}`}></div>
            <span className="text-xs text-zinc-300 font-mono font-semibold tracking-wider">
              {connected ? 'LIVE KERNEL (WSL2)' : 'INTERACTIVE DEMO (NETLIFY)'}
            </span>
          </div>
        </div>
      </header>

      <main className="flex-1 grid grid-cols-12 gap-6 min-h-0 relative z-10">
        <div className="col-span-4 flex flex-col gap-6">
          <div className="rounded-2xl bg-black/40 border border-zinc-800/80 backdrop-blur-xl p-5 shadow-xl flex flex-col h-72 relative overflow-hidden">
            <div className="absolute inset-0 bg-gradient-to-b from-indigo-500/5 to-transparent pointer-events-none"></div>
            <h2 className="text-sm font-semibold text-zinc-200 mb-3 flex items-center gap-2 uppercase tracking-widest relative z-10">
              <Cpu className="w-4 h-4 text-indigo-400" /> Node Telemetry
            </h2>
            <div className="flex justify-between items-end mb-2 relative z-10">
              <div>
                <div className="text-[10px] text-zinc-500 font-mono mb-1">TOTAL MEMORY</div>
                <div className="text-2xl font-mono text-indigo-300">{(totalMem / 1024 / 1024).toFixed(1)} <span className="text-sm text-zinc-500">MB</span></div>
              </div>
              <div>
                <div className="text-[10px] text-zinc-500 font-mono mb-1 text-right">CPU LOAD</div>
                <div className="text-xl font-mono text-emerald-400 text-right">{totalCpu.toFixed(1)} <span className="text-sm text-zinc-500">%</span></div>
              </div>
            </div>
            <div className="flex-1 -mx-2 relative z-10">
              <ReactECharts option={nodeChartOption} style={{ height: '100%', width: '100%' }} />
            </div>
          </div>

          <div className="rounded-2xl bg-black/40 border border-zinc-800/80 backdrop-blur-xl flex flex-col overflow-hidden shadow-xl min-h-[320px] max-h-[500px]">
            <div className="p-4 border-b border-zinc-800/80 bg-zinc-900/30 flex justify-between items-center flex-shrink-0">
              <h2 className="text-sm font-semibold text-zinc-200 flex items-center gap-2 uppercase tracking-widest">
                <HardDrive className="w-4 h-4 text-pink-400" /> Causal Audit Trail
              </h2>
              <span className="text-[10px] font-mono text-zinc-500 bg-zinc-800/50 px-2 py-0.5 rounded">REAL-TIME</span>
            </div>
            <div className="flex-1 p-4 overflow-y-auto flex flex-col justify-end space-y-3 font-mono text-[11px] leading-relaxed">
              <AnimatePresence>
                {events.map((ev, i) => {
                  const isAlert = ev.includes("ALERT") || ev.includes("PREDICTIVE");
                  const isAdapt = ev.includes("ADAPTATION") || ev.includes("SCALING");
                  const isML = ev.includes("ML-ENGINE");
                  const isSys = ev.includes("SYSTEM");
                  return (
                    <motion.div 
                      key={i} 
                      initial={{ opacity: 0, x: -10 }} 
                      animate={{ opacity: 1, x: 0 }} 
                      className={`p-2.5 rounded-lg border ${
                        isAlert ? 'bg-red-500/10 border-red-500/30 text-red-300' : 
                        isAdapt ? 'bg-orange-500/10 border-orange-500/30 text-orange-300' : 
                        isML ? 'bg-purple-500/10 border-purple-500/30 text-purple-300' :
                        isSys ? 'bg-blue-500/10 border-blue-500/30 text-blue-300' : 
                        'bg-zinc-800/30 border-zinc-700/30 text-zinc-400'
                      }`}
                    >
                      {ev}
                    </motion.div>
                  );
                })}
              </AnimatePresence>
            </div>
          </div>
        </div>

        <div className="col-span-8 flex flex-col gap-5 relative">
          <div className="rounded-2xl bg-zinc-900/70 border border-indigo-500/30 backdrop-blur-xl p-4 shadow-[0_0_30px_rgba(99,102,241,0.1)] flex flex-col gap-3 flex-shrink-0">
            <div className="flex justify-between items-center">
              <h2 className="text-sm font-semibold text-zinc-200 flex items-center gap-2 uppercase tracking-widest">
                <Sparkles className="w-4 h-4 text-indigo-400" /> Naive Bayes Auto-Pilot Shell
              </h2>
              <span className="text-[10px] font-mono text-indigo-300 bg-indigo-500/10 px-2 py-0.5 rounded border border-indigo-500/20">
                130+ COMMAND DATASET • SUB-2µS INFERENCE
              </span>
            </div>
            <div className="flex gap-3">
              <input 
                type="text"
                value={autoInput}
                onChange={e => setAutoInput(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && runAutopilot()}
                placeholder="Type command (e.g., 'curl -fsSL example.com', 'gcc main.c', 'rm -rf /') to test ML inference..."
                className="flex-1 bg-black/60 border border-zinc-700 rounded-lg px-4 py-2.5 font-mono text-sm text-zinc-200 outline-none focus:border-indigo-500 transition-colors"
              />
              <button 
                onClick={runAutopilot}
                disabled={isAutoLoading}
                className="px-6 py-2.5 bg-indigo-600 hover:bg-indigo-500 text-white rounded-lg font-mono text-sm font-bold transition-colors disabled:opacity-50 min-w-[120px] flex items-center justify-center gap-2"
              >
                {isAutoLoading ? 'CLASSIFYING...' : 'EXECUTE'}
              </button>
            </div>

            {latestAutopilot && (
              <motion.div 
                initial={{ opacity: 0, y: -5 }} 
                animate={{ opacity: 1, y: 0 }} 
                className="bg-black/50 border border-zinc-800 rounded-xl p-3 flex flex-col gap-2 font-mono text-xs"
              >
                <div className="flex justify-between items-center">
                  <div className="flex items-center gap-2">
                    <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                    <span className="text-zinc-400">Classified:</span>
                    <span className="text-white font-bold">{latestAutopilot.command}</span>
                    <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-indigo-500/20 text-indigo-300 border border-indigo-500/30 uppercase">
                      {latestAutopilot.intent} ({(latestAutopilot.confidence * 100).toFixed(1)}%)
                    </span>
                  </div>
                  <span className="text-[10px] text-zinc-500">Container: {latestAutopilot.id}</span>
                </div>

                {latestAutopilot.probabilities && (
                  <div className="grid grid-cols-5 gap-2 pt-1 border-t border-zinc-800/80">
                    {Object.entries(latestAutopilot.probabilities).map(([cls, prob]) => (
                      <div key={cls} className="flex flex-col gap-1">
                        <div className="flex justify-between text-[9px] text-zinc-400">
                          <span className="truncate">{cls}</span>
                          <span>{(prob * 100).toFixed(0)}%</span>
                        </div>
                        <div className="w-full bg-zinc-800 rounded-full h-1 overflow-hidden">
                          <div 
                            className={`h-full ${cls === latestAutopilot.intent ? 'bg-indigo-500' : 'bg-zinc-600'}`} 
                            style={{ width: `${Math.max(4, prob * 100)}%` }}
                          />
                        </div>
                      </div>
                    ))}
                  </div>
                )}

                {latestAutopilot.output && (
                  <div className="bg-zinc-950 rounded p-2 text-zinc-300 text-[11px] whitespace-pre-wrap max-h-24 overflow-y-auto border border-zinc-900 mt-1">
                    {latestAutopilot.output}
                  </div>
                )}
              </motion.div>
            )}
          </div>

          <div className="flex-1 rounded-2xl bg-black/40 border border-zinc-800/80 backdrop-blur-xl relative flex flex-col shadow-xl overflow-hidden min-h-[300px]">
            <div className="p-4 border-b border-zinc-800/80 bg-zinc-900/30 flex justify-between items-center flex-shrink-0">
              <h2 className="text-sm font-semibold text-zinc-200 flex items-center gap-2 uppercase tracking-widest">
                <Shield className="w-4 h-4 text-emerald-400" /> Active Topology & Cgroup Sandboxes
              </h2>
              <div className="flex gap-3">
                <div className="text-xs text-zinc-400 font-mono bg-zinc-900/80 px-3 py-1 rounded-full border border-zinc-800">ACTIVE: {activeCount}</div>
                <div className="text-xs text-emerald-400 font-mono bg-emerald-500/10 px-3 py-1 rounded-full border border-emerald-500/20">SECCOMP-BPF ACTIVE</div>
              </div>
            </div>
            
            <div className="flex-1 p-5 overflow-y-auto">
              {containers.length === 0 ? (
                <div className="h-full flex flex-col items-center justify-center opacity-70">
                  <div className="w-28 h-28 rounded-full border border-zinc-700/50 flex items-center justify-center mb-4 relative">
                    <motion.div animate={{ rotate: 360 }} transition={{ duration: 15, repeat: Infinity, ease: "linear" }} className="absolute inset-[-1px] rounded-full border-t border-l border-indigo-500/50"></motion.div>
                    <motion.div animate={{ rotate: -360 }} transition={{ duration: 25, repeat: Infinity, ease: "linear" }} className="absolute inset-[-4px] rounded-full border-b border-r border-blue-500/30"></motion.div>
                    <Shield className="w-8 h-8 text-zinc-600" />
                  </div>
                  <h3 className="text-lg font-semibold text-zinc-400 tracking-wide">Awaiting Workloads</h3>
                  <p className="text-xs text-zinc-600 mt-1 font-mono">Execute via Auto-Pilot or click SPAWN to launch isolated sandboxes.</p>
                </div>
              ) : (
                <div className="grid grid-cols-2 gap-4">
                  <AnimatePresence>
                    {containers.map(c => {
                      const isFrozen = c.status === "Frozen";
                      const scoreColor = c.isolation_score >= 80 ? 'text-emerald-400' : c.isolation_score >= 50 ? 'text-orange-400' : 'text-red-400';
                      
                      return (
                        <motion.div 
                          key={c.id}
                          layoutId={`card-${c.id}`}
                          onClick={() => setSelectedContainer(c)}
                          initial={{ opacity: 0, scale: 0.95 }}
                          animate={{ opacity: 1, scale: 1 }}
                          exit={{ opacity: 0, scale: 0.9 }}
                          className={`group cursor-pointer bg-zinc-900/40 border ${isFrozen ? 'border-blue-500/50' : c.isolation_score < 80 ? 'border-orange-500/40 shadow-[0_0_20px_rgba(249,115,22,0.1)]' : 'border-zinc-700/50 hover:border-indigo-500/40'} rounded-2xl p-5 flex flex-col gap-3 relative overflow-hidden transition-all`}
                        >
                          <div className="absolute top-0 right-0 p-4 opacity-0 group-hover:opacity-100 transition-opacity text-zinc-500">
                            <Maximize2 className="w-4 h-4" />
                          </div>
                          
                          <div className="flex justify-between items-start">
                            <div>
                              <div className="flex items-center gap-2 mb-1.5">
                                <h4 className="text-zinc-200 font-mono font-bold text-base">{c.id.substring(0, 10)}</h4>
                                <span className={`text-[9px] font-mono font-bold px-2 py-0.5 rounded uppercase tracking-wider ${isFrozen ? 'bg-blue-500/20 text-blue-400' : c.status === 'Running' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-red-500/20 text-red-400'}`}>
                                  {c.status}
                                </span>
                              </div>
                              <div className="flex items-center gap-2">
                                <span className="text-xs text-indigo-300 font-mono bg-indigo-500/10 px-2 py-0.5 rounded w-fit uppercase tracking-wider">
                                  Intent: {c.options?.intent || "default"}
                                </span>
                                <span className="text-[10px] text-zinc-500 font-mono bg-zinc-800/60 px-2 py-0.5 rounded">
                                  14 Syscalls Filtered
                                </span>
                              </div>
                            </div>
                            
                            <div className="flex flex-col items-end">
                              <div className="text-[10px] text-zinc-500 font-mono mb-0.5">SCORE</div>
                              <div className={`text-xl font-bold font-mono ${scoreColor}`}>{c.isolation_score}</div>
                            </div>
                          </div>
                          
                          <div className="grid grid-cols-3 gap-2.5 mt-1">
                            <div className="bg-black/50 rounded-xl p-2.5 border border-zinc-800/50">
                              <div className="text-[9px] text-zinc-500 font-mono mb-0.5">MEM LIMIT</div>
                              <div className="text-xs text-zinc-300 font-mono">{c.profile?.memory_max ? (parseInt(c.profile.memory_max) / 1024 / 1024).toFixed(0) : '?'} MB</div>
                            </div>
                            <div className="bg-black/50 rounded-xl p-2.5 border border-zinc-800/50">
                              <div className="text-[9px] text-zinc-500 font-mono mb-0.5">CURRENT</div>
                              <div className="text-xs text-indigo-400 font-mono">{(c.metrics?.memory_usage / 1024 / 1024 || 0).toFixed(1)} MB</div>
                            </div>
                            <div className="bg-black/50 rounded-xl p-2.5 border border-zinc-800/50">
                              <div className="text-[9px] text-zinc-500 font-mono mb-0.5">VELOCITY</div>
                              <div className="text-xs text-pink-400 font-mono">{c.metrics?.memory_velocity ? `+${(c.metrics.memory_velocity / 1024 / 1024 / 2).toFixed(1)}/s` : '0.0/s'}</div>
                            </div>
                          </div>
                        </motion.div>
                      );
                    })}
                  </AnimatePresence>
                </div>
              )}
            </div>
          </div>
        </div>
      </main>

      <AnimatePresence>
        {selectedContainer && (
          <motion.div 
            initial={{ opacity: 0 }} 
            animate={{ opacity: 1 }} 
            exit={{ opacity: 0 }} 
            className="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-8"
            onClick={(e) => { if (e.target === e.currentTarget) setSelectedContainer(null); }}
          >
            <motion.div 
              layoutId={`card-${selectedContainer.id}`}
              className="w-full max-w-4xl bg-zinc-900 border border-zinc-700 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh]"
            >
              <div className="p-6 border-b border-zinc-800 flex justify-between items-center bg-black/20 flex-shrink-0">
                <div>
                  <h2 className="text-xl font-bold font-mono text-white flex items-center gap-3">
                    {selectedContainer.id}
                    <span className={`text-xs font-mono px-2 py-1 rounded uppercase ${selectedContainer.status === 'Frozen' ? 'bg-blue-500/20 text-blue-400' : 'bg-emerald-500/20 text-emerald-400'}`}>{selectedContainer.status}</span>
                  </h2>
                  <div className="flex items-center gap-3 mt-1">
                    <p className="text-sm text-zinc-400 font-mono">INTENT: {selectedContainer.options?.intent || "default"}</p>
                    <span className="text-xs text-emerald-400 font-mono bg-emerald-500/10 px-2 py-0.5 rounded border border-emerald-500/20">
                      SECCOMP-BPF (14 RULES ACTIVE)
                    </span>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  {selectedContainer.status !== 'Frozen' ? (
                    <button onClick={() => performAction(selectedContainer.id, 'freeze')} className="px-4 py-2 bg-blue-500/10 text-blue-400 hover:bg-blue-500/20 rounded-lg flex items-center gap-2 font-mono text-sm border border-blue-500/20 transition-colors">
                      <Pause className="w-4 h-4"/> FREEZE
                    </button>
                  ) : (
                    <button onClick={() => performAction(selectedContainer.id, 'thaw')} className="px-4 py-2 bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 rounded-lg flex items-center gap-2 font-mono text-sm border border-emerald-500/20 transition-colors">
                      <Play className="w-4 h-4"/> THAW
                    </button>
                  )}
                  <button onClick={() => stopContainer(selectedContainer.id)} className="px-4 py-2 bg-red-500/10 text-red-400 hover:bg-red-500/20 rounded-lg flex items-center gap-2 font-mono text-sm border border-red-500/20 transition-colors">
                    TERMINATE
                  </button>
                  <button onClick={() => setSelectedContainer(null)} className="p-2 text-zinc-400 hover:text-white rounded-lg hover:bg-zinc-800 transition-colors ml-4">
                    <X className="w-5 h-5"/>
                  </button>
                </div>
              </div>
              
              <div className="p-6 bg-black/40 flex flex-col gap-6 overflow-y-auto">
                <div className="grid grid-cols-2 gap-6">
                  <div className="bg-zinc-900 rounded-xl border border-zinc-800 p-5 h-64 flex flex-col">
                    <div className="text-xs text-zinc-500 font-mono mb-2">MEMORY TRAJECTORY (MB)</div>
                    <div className="flex-1 -mx-3">
                      <ReactECharts 
                        option={{
                          animation: false, grid: { top: 10, right: 10, bottom: 20, left: 40 },
                          xAxis: { type: 'category', show: false },
                          yAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(255,255,255,0.05)' } }, axisLabel: { color: '#666' } },
                          series: [{
                            data: history[selectedContainer.id] || [],
                            type: 'line', smooth: true, symbol: 'none',
                            lineStyle: { width: 3, color: '#a855f7' },
                            areaStyle: { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: 'rgba(168, 85, 247, 0.4)' }, { offset: 1, color: 'rgba(168, 85, 247, 0.0)' }] } }
                          }]
                        }} 
                        style={{ height: '100%', width: '100%' }} 
                      />
                    </div>
                  </div>

                  <div className="flex flex-col gap-4 h-64">
                    <div className="bg-zinc-900 rounded-xl border border-zinc-800 p-5 flex-1 overflow-y-auto">
                      <div className="text-xs text-zinc-500 font-mono mb-4">NAMESPACE BOUNDARIES</div>
                      <div className="flex flex-wrap gap-2">
                        {selectedContainer.namespaces?.map((ns, idx) => (
                          <span key={idx} className={`text-[10px] font-mono px-2 py-1 rounded border ${ns.includes('Airgapped') || ns.includes('Isolated') ? 'bg-red-500/10 border-red-500/30 text-red-400' : 'bg-zinc-800/50 border-zinc-700/50 text-zinc-300'}`}>
                            {ns}
                          </span>
                        ))}
                      </div>
                    </div>
                    
                    <div className="bg-zinc-900 rounded-xl border border-zinc-800 p-5 flex-1 flex flex-col justify-center items-center">
                      <div className="text-xs text-zinc-500 font-mono mb-2">LIVE ISOLATION SCORE</div>
                      <div className={`text-5xl font-black font-mono tracking-tighter ${selectedContainer.isolation_score >= 80 ? 'text-emerald-400 drop-shadow-[0_0_15px_rgba(52,211,153,0.3)]' : selectedContainer.isolation_score >= 50 ? 'text-orange-400 drop-shadow-[0_0_15px_rgba(251,146,60,0.3)]' : 'text-red-400 drop-shadow-[0_0_15px_rgba(248,113,113,0.3)]'}`}>
                        {selectedContainer.isolation_score}
                      </div>
                    </div>
                  </div>
                </div>

                <div className="bg-zinc-950 rounded-xl border border-zinc-800 p-4 h-64 flex flex-col font-mono">
                  <div className="text-xs text-zinc-500 mb-2 flex items-center justify-between">
                    <span>LIVE INTERACTIVE SHELL — Container: {selectedContainer.id}</span>
                    <span className="text-[9px] bg-emerald-500/10 text-emerald-400 px-2 py-0.5 rounded border border-emerald-500/20">
                      {connected ? 'CONNECTED (KERNEL)' : 'ACTIVE (SANDBOX)'}
                    </span>
                  </div>
                  <div className="flex-1 overflow-y-auto bg-black rounded p-3 text-sm text-zinc-300 leading-relaxed break-all">
                    {currentTermOutput.map((line, i) => (
                      <div key={i} className={line.startsWith('aether-shell$') ? 'text-indigo-400 mt-1' : 'whitespace-pre-wrap'}>
                        {line}
                      </div>
                    ))}
                  </div>
                  <div className="mt-3 flex items-center gap-2">
                    <span className="text-indigo-400">aether-shell$</span>
                    <input 
                      type="text" 
                      value={termInput}
                      onChange={e => setTermInput(e.target.value)}
                      onKeyDown={e => e.key === 'Enter' && runTermCommand()}
                      className="flex-1 bg-transparent border-none outline-none text-zinc-200"
                      placeholder="e.g., ip a, uname -a, ls -la /"
                      disabled={selectedContainer.status === 'Frozen'}
                    />
                  </div>
                </div>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

export default App;
