package cloudtask

import (
	"context"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	cloudtaskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	gax "github.com/googleapis/gax-go/v2"
)

type TaskClient interface {
	CreateTask(ctx context.Context, req *cloudtaskspb.CreateTaskRequest, opts ...gax.CallOption) (*cloudtaskspb.Task, error)
}

var _ TaskClient = (*cloudtasks.Client)(nil)
