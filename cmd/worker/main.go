package main

import (
	"log"
)

func main() {
	log.Println("Starting worker...")
	// In a real implementation, this worker would connect to the controller
	// to receive workloads. For now, it just starts and waits.
	select {}
}
