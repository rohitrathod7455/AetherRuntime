#!/bin/bash

# AetherRuntime v2.0 - Automated Demo Script
# This script simulates a workload sequence for a live presentation/viva.

echo "=========================================================="
echo " AetherRuntime v2.0 : Intent-Driven Adaptive Isolation Demo"
echo "=========================================================="
echo "Pre-flight checks:"
echo " 1. Is the Go daemon (aetherd) running in Terminal 1 as root?"
echo " 2. Is the React dashboard running in Terminal 2 (http://localhost:5173)?"
echo "=========================================================="
read -p "Press [Enter] to begin the demonstration..."

echo -e "\n\033[1;34m[STEP 1]\033[0m Spawning a Trusted Batch Job..."
echo "Intent: 'batch job'"
curl -s -X POST -H "Content-Type: application/json" \
     -d '{"command": "echo \"Processing data...\" && sleep 45", "intent": "batch job"}' \
     http://localhost:8080/api/containers
echo ""
echo "-> Check the dashboard: generous memory ceiling (1000MB), high PID limit, score 100."
read -p "Press [Enter] for Step 2..."

echo -e "\n\033[1;31m[STEP 2]\033[0m Spawning an Untrusted Script..."
echo "Intent: 'untrusted script'"
curl -s -X POST -H "Content-Type: application/json" \
     -d '{"command": "echo \"Running unknown payload...\" && sleep 45", "intent": "untrusted script"}' \
     http://localhost:8080/api/containers
echo ""
echo "-> Check the dashboard: locked-down boundaries (50MB, 10 PIDs), Seccomp-BPF active."
read -p "Press [Enter] for Step 3..."

echo -e "\n\033[1;35m[STEP 3]\033[0m Testing Bayesian ML Auto-Pilot Inference..."
echo "Executing: 'curl -fsSL https://get.docker.com'"
curl -s -X POST -H "Content-Type: application/json" \
     -d '{"command": "curl -fsSL https://get.docker.com"}' \
     http://localhost:8080/api/autopilot | grep -o '"intent":"[^"]*","namespaces":\[[^]]*\]'
echo ""
echo "-> Notice: The ML Engine automatically detected 'network-task' and configured namespaces without manual user flags!"
read -p "Press [Enter] for Step 4..."

echo -e "\n\033[1;33m[STEP 4]\033[0m Testing Malicious/Dangerous Command Isolation..."
echo "Executing: 'rm -rf / --no-preserve-root'"
curl -s -X POST -H "Content-Type: application/json" \
     -d '{"command": "rm -rf / --no-preserve-root"}' \
     http://localhost:8080/api/autopilot | grep -o '"intent":"[^"]*"'
echo ""
echo "-> Notice: Classified as 'untrusted-dangerous' with maximum namespace isolation and 14 Seccomp syscall blocks!"
echo "=========================================================="
echo "Demo Sequence Complete. Check the Dashboard for real-time telemetry."
