package agent

import (
	"context"
	"corithator/internal/orchestrator"
	pb "corithator/proto"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type AgentConfig struct {
	OrchestratorURI      string //если пустой - меняется в :8081
	MaxWorkers           int    //минимум 1
	DelayMs              int    //задержка между запросами задач
	TimeAdditionMs       int
	TimeSubstractionMs   int
	TimeMultiplicationMs int
	TimeDivisionMs       int
}

type Agent struct {
	Config     AgentConfig
	tasks      chan orchestrator.Task
	grpcClient pb.InternalServiceClient
}

func NewAgent(config AgentConfig) *Agent {
	if config.OrchestratorURI == "" {
		config.OrchestratorURI = "localhost:8081"
	}
	if config.DelayMs < 0 {
		config.DelayMs = 0
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

	addr := a.Config.OrchestratorURI

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		log.Println("could not connect to grpc server: ", err)
		os.Exit(1)
	}

	defer conn.Close()

	a.grpcClient = pb.NewInternalServiceClient(conn)

	for {
		task, err := a.getTask()
		if err != nil {
			if err.Error() != "rpc error: code = Unknown desc = no tasks available" {
				slog.Error(fmt.Sprint("[AGENT]: Error getting task: ", err))
			} else {
				slog.Debug("No new tasks")
			}
		} else {
			a.tasks <- task
		}
		time.Sleep(time.Millisecond * time.Duration(a.Config.DelayMs))
	}
}

func (a *Agent) sendResult(id int, result float64) {
	a.grpcClient.TaskPost(context.TODO(),
		&pb.TaskPostRequest{Id: int32(id), Result: float32(result)})
}

func (a *Agent) getTask() (orchestrator.Task, error) {
	data, err := a.grpcClient.TaskGet(context.TODO(), &pb.TaskGetRequest{})
	if err != nil {
		return orchestrator.Task{}, err
	}
	task := orchestrator.Task{Arg1: float64(data.Arg1),
		Arg2:      float64(data.Arg2),
		Id:        int(data.Id),
		Operation: data.Operation}
	return task, nil
}
