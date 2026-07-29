package queue

import (
	"github.com/hibiken/asynq"
)

func RedisOpt(addr string, db int) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: addr, DB: db}
}

type Client struct {
	inner *asynq.Client
}

func NewClient(addr string, db int) *Client {
	return &Client{inner: asynq.NewClient(RedisOpt(addr, db))}
}

func (c *Client) Enqueue(t *asynq.Task, opts ...asynq.Option) error {
	_, err := c.inner.Enqueue(t, opts...)
	return err
}

func (c *Client) Close() error {
	return c.inner.Close()
}

// NewServer configures worker concurrency per queue. Media is deliberately
// throttled because transcode and vision dominate cost and CPU (doc bab 23.2).
func NewServer(addr string, db int) *asynq.Server {
	return asynq.NewServer(RedisOpt(addr, db), asynq.Config{
		Concurrency: 6,
		Queues: map[string]int{
			QueueCritical: 4,
			QueueDefault:  3,
			QueueMedia:    2,
			QueueLow:      1,
		},
	})
}
