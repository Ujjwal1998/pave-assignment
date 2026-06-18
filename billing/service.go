package billing

import (
	"context"
	"fmt"

	"encore.dev"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"pave-bank/activity"
	"pave-bank/workflow"
)

//encore:service
type Service struct {
	temporalClient client.Client
	temporalWorker worker.Worker
	taskQueue      string
}

func initService() (*Service, error) {
	tc, err := client.Dial(client.Options{HostPort: cfg.TemporalServer})
	if err != nil {
		return nil, fmt.Errorf("create temporal client: %w", err)
	}

	activity.Init(newActivityStore())

	queue := encore.Meta().Environment.Name + "-" + workflow.BaseTaskQueueName
	w := worker.New(tc, queue, worker.Options{})
	w.RegisterWorkflow(workflow.BillWorkflow)
	w.RegisterActivity(activity.UpdateBillClosed)

	if err := w.Start(); err != nil {
		tc.Close()
		return nil, fmt.Errorf("start temporal worker: %w", err)
	}

	return &Service{temporalClient: tc, temporalWorker: w, taskQueue: queue}, nil
}

func (s *Service) Shutdown(force context.Context) {
	s.temporalWorker.Stop()
	s.temporalClient.Close()
}
