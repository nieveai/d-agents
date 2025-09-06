package main

import (
	"log"

	"github.com/nieveai/d-agents/internal/worker"
)

func main() {
	log.Println("Starting shopping agent worker...")
	// In a real implementation, this worker would connect to the controller
	// to receive workloads. For now, it just starts and waits.
	worker.ProcessWorkload( /* workload for shopping agent */ )
	log.Println("Shopping agent worker finished.")
}