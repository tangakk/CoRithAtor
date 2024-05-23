package main

import (
	"corithator/internal/agent"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var maxWorkers = *flag.Int("COMPUTING_POWER", 1, "Кол-во горутин, параллельно считающих числа")

func main() {
	var config agent.AgentConfig

	pathToConfig, _ := filepath.Abs("../CoRithAtor/settings/agent.json")
	f, err := os.Open(pathToConfig)
	if err != nil {
		fmt.Println("error opening agent.json, continuing without")
		fmt.Println(err)
	} else {
		json.NewDecoder(f).Decode(&config)
	}

	if maxWorkers != 1 {
		config.MaxWorkers = maxWorkers
	}

	agent := agent.NewAgent(config)

	agent.Run()
}
