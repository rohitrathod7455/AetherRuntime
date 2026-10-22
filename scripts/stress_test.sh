#!/bin/bash

echo "=========================================================="
echo " AetherRuntime : REAL KERNEL STRESS TEST"
echo "=========================================================="
echo "This script proves the engine is NOT a simulation by forcing"
echo "real memory allocation inside the isolated container to trigger"
echo "the Predictive Engine and cgroup throttling."
echo "=========================================================="
read -p "Press [Enter] to inject a massive memory spike into an Untrusted Script..."

# We use awk to rapidly allocate arrays in memory inside the Alpine rootfs.
# This will quickly consume ~40MB+ of RAM, triggering the predictive velocity governor.
curl -s -X POST -H "Content-Type: application/json" \
     -d '{"command": "awk '\''BEGIN{ print \"Allocating RAM...\"; for(i=0;i<1000000;i++) a[i]=\"massive_string_\"i; print \"Holding RAM...\"; system(\"sleep 60\")}'\''", "intent": "untrusted script"}' \
     http://localhost:8080/api/containers | grep -o '"status":"[^"]*"'

echo "-> IMMEDIATELY check the Dashboard!"
echo "-> You will see the MEM CURRENT spike rapidly."
echo "-> You will see the VELOCITY jump to +X MB/s."
echo "-> The Engine will detect this and fire a PREDICTIVE ALERT."
