package main

import "corithator/internal/orchestrator"

func main() {
	var config = orchestrator.OrchestratorConfig{Port: 8080}

	orchestrator := orchestrator.NewOrchestrator(config)

	orchestrator.Run()
}
