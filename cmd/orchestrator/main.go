package main

import (
	"corithator/internal/orchestrator"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var port = *flag.Int("port", 8080, "порт, где будет висеть оркестратор")

func main() {
	var config orchestrator.OrchestratorConfig

	pathToConfig, _ := filepath.Abs("../CoRithAtor/settings/orchestrator.json")
	f, err := os.Open(pathToConfig)
	if err != nil {
		fmt.Println("error opening orhectrator.json, continuing without")
		fmt.Println(err)
	} else {
		json.NewDecoder(f).Decode(&config)
	}

	if port != 8080 {
		config.Port = port
	}

	orchestrator := orchestrator.NewOrchestrator(config)

	orchestrator.Run()
}
