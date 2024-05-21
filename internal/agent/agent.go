package agent

import (
	"bytes"
	"corithator/internal/orchestrator"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type AgentConfig struct {
	OrchestratorURI      string //если пустой - меняется в :8080
	TaskPath             string //по-муолчанию - /internal/task
	MaxWorkers           int    //минимум 1
	DelaySeconds         int    //задержка между запросами задач
	TimeAdditionMs       int
	TimeSubstractionMs   int
	TimeMultiplicationMs int
	TimeDivisionMs       int
}

type Agent struct {
	Config AgentConfig
	tasks  chan orchestrator.Task
}

func NewAgent(config AgentConfig) *Agent {
	if config.OrchestratorURI == "" {
		config.OrchestratorURI = "http://localhost:8080"
	}
	if config.TaskPath == "" {
		config.TaskPath = "/internal/task"
	}
	if config.DelaySeconds < 0 {
		config.DelaySeconds = 0
	}
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 1
	}
	if config.TimeAdditionMs < 0 {
		config.TimeAdditionMs = 0
	}
	if config.TimeSubstractionMs < 0 {
		config.TimeSubstractionMs = 0
	}
	if config.TimeMultiplicationMs < 0 {
		config.TimeMultiplicationMs = 0
	}
	if config.TimeDivisionMs < 0 {
		config.TimeDivisionMs = 0
	}
	return &Agent{Config: config, tasks: make(chan orchestrator.Task)}
}

func (a *Agent) Run() {
	for range a.Config.MaxWorkers {
		go func() {
			for task := range a.tasks {
				switch task.Operation {
				case "+":
					time.Sleep(time.Duration(a.Config.TimeAdditionMs) * time.Millisecond)
					a.sendResult(task.Id, task.Arg1+task.Arg2)
				case "-":
					time.Sleep(time.Millisecond * time.Duration(a.Config.TimeSubstractionMs))
					a.sendResult(task.Id, task.Arg1-task.Arg2)
				case "*":
					time.Sleep(time.Millisecond * time.Duration(a.Config.TimeMultiplicationMs))
					a.sendResult(task.Id, task.Arg1*task.Arg2)
				case "/":
					time.Sleep(time.Millisecond * time.Duration(a.Config.TimeDivisionMs))
					a.sendResult(task.Id, task.Arg1/task.Arg2)
				}
			}
		}()
	}
	go func() {
		for {
			task, err := a.getTask()
			if err != nil {
				if err.Error() != "no tasks available" {
					slog.Error("Error getting task: ", err)
				} else {
					slog.Debug("No new tasks")
				}
			} else {
				a.tasks <- task
			}
			time.Sleep(time.Second * time.Duration(a.Config.DelaySeconds))
		}
	}()
}

func (a *Agent) sendResult(id int, result float64) {
	data := orchestrator.TaskPostRequest{Id: id, Result: result}
	jsonValue, _ := json.Marshal(data)
	http.Post(a.Config.OrchestratorURI+a.Config.TaskPath, "application/json",
		bytes.NewBuffer(jsonValue))
}

func (a *Agent) getTask() (orchestrator.Task, error) {
	resp, err := http.Get(a.Config.OrchestratorURI + a.Config.TaskPath)
	if err != nil {
		return orchestrator.Task{}, err
	}

	if resp.StatusCode != 200 {
		return orchestrator.Task{}, errors.New("no tasks available")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return orchestrator.Task{}, err
	}

	var t orchestrator.TaskGetResponse
	json.Unmarshal(body, &t)
	return t.Task, nil
}
