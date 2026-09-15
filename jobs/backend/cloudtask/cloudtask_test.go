package cloudtask_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	cloudtaskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/stretchr/testify/require"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/cloudtask"
	"github.com/asimmons91/trails/pack"
)

func TestEnqueue_CreatesTaskWithExpectedFields(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig())

	err := b.Enqueue(context.Background(), jobs.Enqueued{
		Kind:  "greet",
		Args:  json.RawMessage(`{"name":"ada"}`),
		Queue: "greetings",
	})
	require.NoError(t, err)

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, "greet", row.Kind)
	require.Equal(t, `{"name":"ada"}`, row.Args)
	require.Equal(t, "greetings", row.Queue)
	require.Equal(t, "pending", row.State)
	require.Equal(t, jobs.DefaultMaxAttempts, row.MaxAttempts)
	require.NotEmpty(t, row.TaskName, "Enqueue should record the Cloud Tasks task name it got back")

	req := client.firstRequest()
	require.NotNil(t, req)
	require.Equal(t, "projects/test-project/locations/us-central1/queues/greetings", req.Parent)

	httpReq := req.Task.GetHttpRequest()
	require.NotNil(t, httpReq)
	require.Equal(t, testBaseURL+"/tasks/run", httpReq.Url)
	require.Equal(t, cloudtaskspb.HttpMethod_POST, httpReq.HttpMethod)

	var payload struct {
		JobID int64 `json:"job_id"`
	}
	require.NoError(t, json.Unmarshal(httpReq.Body, &payload))
	require.Equal(t, row.ID, payload.JobID)

	oidc := httpReq.GetOidcToken()
	require.NotNil(t, oidc)
	require.Equal(t, testServiceAccount, oidc.ServiceAccountEmail)
	require.Equal(t, testBaseURL, oidc.Audience)

	require.Nil(t, req.Task.ScheduleTime, "ScheduleTime should be unset when Enqueued.ScheduledAt is zero")
}

func TestEnqueue_DefaultsEmptyQueueName(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig())

	require.NoError(t, b.Enqueue(context.Background(), jobs.Enqueued{Kind: "k", Args: json.RawMessage(`{}`)}))

	req := client.firstRequest()
	require.NotNil(t, req)
	require.Equal(t, "projects/test-project/locations/us-central1/queues/default", req.Parent)
}

func TestEnqueue_SetsScheduleTimeWhenDelayed(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig())

	scheduledAt := time.Now().Add(10 * time.Minute)
	err := b.Enqueue(context.Background(), jobs.Enqueued{
		Kind:        "k",
		Args:        json.RawMessage(`{}`),
		ScheduledAt: scheduledAt,
	})
	require.NoError(t, err)

	req := client.firstRequest()
	require.NotNil(t, req)
	require.NotNil(t, req.Task.ScheduleTime)
	require.WithinDuration(t, scheduledAt, req.Task.ScheduleTime.AsTime(), time.Second)
}

func TestEnqueue_CreateTaskFailure_DeletesOrphanedRowAndReturnsError(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{err: errors.New("cloud tasks unavailable")}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig())

	err := b.Enqueue(context.Background(), jobs.Enqueued{Kind: "k", Args: json.RawMessage(`{}`)})
	require.Error(t, err)

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Empty(t, rows, "the local row should not be left orphaned when CreateTask fails")
}

func TestEnqueue_HonorsCustomQueueNameMapping(t *testing.T) {
	db := newTestDB(t)
	client := &fakeTaskClient{}
	b := cloudtask.New(db, jobs.NewRegistry(), client, testConfig(),
		cloudtask.WithQueueName(func(logical string) string { return "mapped-" + logical }))

	require.NoError(t, b.Enqueue(context.Background(), jobs.Enqueued{Kind: "k", Args: json.RawMessage(`{}`), Queue: "urgent"}))

	req := client.firstRequest()
	require.NotNil(t, req)
	require.Equal(t, "projects/test-project/locations/us-central1/queues/mapped-urgent", req.Parent)
}

func TestClose_IsANoOp(t *testing.T) {
	db := newTestDB(t)
	b := cloudtask.New(db, jobs.NewRegistry(), &fakeTaskClient{}, testConfig())
	require.NoError(t, b.Close())
}
