package main

import (
	"fmt"
	"log"
	"os"

	"aether-runtime/control-plane"
	"aether-runtime/runtime"
)

const version = "2.0.0"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "child" {
		id := os.Args[2]
		cmd := os.Args[3]
		rootfs := os.Args[4]
		intent := os.Args[5]
		if err := runtime.Child(id, cmd, rootfs, intent); err != nil {
			log.Fatalf("Child process failed: %v", err)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("AetherRuntime v%s\n", version)
		return
	}

	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║           AetherRuntime v2.0 — Adaptive Isolation       ║")
	fmt.Println("║  Intent-Driven | Predictive Governor | Seccomp-BPF      ║")
	fmt.Println("║  Naive Bayes ML Classifier (130+ labeled commands)      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	
	if os.Geteuid() != 0 {
		log.Println("⚠  WARNING: aetherd must run as root for full namespace/seccomp support.")
	}

	rootfs := "./rootfs"
	os.MkdirAll(rootfs, 0755)
	
	engine := runtime.NewEngine(rootfs)
	log.Printf("[Runtime] Engine initialized — ML classifier loaded, Seccomp filter ready")
	log.Printf("[Runtime] Vocabulary size: %d tokens across 5 intent classes", engine.Classifier.VocabSize)
	
	api := controlplane.NewServer(engine)
	
	log.Fatal(api.Start(8080))
}
