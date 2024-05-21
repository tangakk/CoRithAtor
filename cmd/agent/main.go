package main

import "corithator/internal/agent"

func main() {
	var config = agent.AgentConfig{DelaySeconds: 1,
		TimeAdditionMs:       2000,
		TimeSubstractionMs:   2000,
		TimeMultiplicationMs: 3000,
		MaxWorkers:           2}

	agent := agent.NewAgent(config)

	agent.Run()
	for {
	}
}
