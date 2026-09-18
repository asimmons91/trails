package cloudtask

import (
	"context"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	cloudtaskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	gax "github.com/googleapis/gax-go/v2"
)

// TaskClient is the subset of *cloudtasks.Client this package needs, so
// tests can substitute a fake in place of a real Cloud Tasks connection.
type TaskClient interface {
	// CreateTask creates req's task on a Cloud Tasks queue.
	CreateTask(ctx context.Context, req *cloudtaskspb.CreateTaskRequest, opts ...gax.CallOption) (*cloudtaskspb.Task, error)
}

var _ TaskClient = (*cloudtasks.Client)(nil)
