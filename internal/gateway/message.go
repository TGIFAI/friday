package gateway

import (
	"context"
	"sync"

	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/pkg/logs"
)

type QueueOptions struct {
	LaneBuffer    int
	MaxConcurrent int
}

type MessageQueue struct {
	lanes         map[string]chan *channel.Message
	mu            sync.RWMutex
	handler       func(context.Context, *channel.Message) error
	ctx           context.Context
	laneBuffer    int
	maxConcurrent chan struct{}
}

func newMessageQueue(opts QueueOptions) *MessageQueue {
	laneBuffer := opts.LaneBuffer
	if laneBuffer <= 0 {
		laneBuffer = 10
	}

	maxConcurrent := opts.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 100
	}

	return &MessageQueue{
		lanes:         make(map[string]chan *channel.Message),
		laneBuffer:    laneBuffer,
		maxConcurrent: make(chan struct{}, maxConcurrent),
	}
}

func (q *MessageQueue) Init(ctx context.Context, handler func(context.Context, *channel.Message) error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ctx = ctx
	q.handler = handler
	return nil
}

func (q *MessageQueue) Enqueue(ctx context.Context, msg *channel.Message) error {
	lane := q.getOrCreateLane(msg.SessionKey)
	select {
	case lane <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *MessageQueue) getOrCreateLane(sessionKey string) chan *channel.Message {
	q.mu.RLock()
	lane, exists := q.lanes[sessionKey]
	q.mu.RUnlock()
	if exists {
		return lane
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if lane, exists := q.lanes[sessionKey]; exists {
		return lane
	}

	lane = make(chan *channel.Message, q.laneBuffer)
	q.lanes[sessionKey] = lane
	go q.processLane(sessionKey, lane)
	return lane
}

func (q *MessageQueue) processLane(sessionKey string, lane chan *channel.Message) {
	for {
		select {
		case <-q.ctx.Done():
			return
		case msg := <-lane:
			if err := q.acquire(q.ctx); err != nil {
				return
			}
			err := q.handler(q.ctx, msg)
			q.release()
			if err != nil {
				logs.CtxWarn(q.ctx, "[queue] failed to process message in lane %s: %v", sessionKey, err)
			}
		}
	}
}

func (q *MessageQueue) acquire(ctx context.Context) error {
	select {
	case q.maxConcurrent <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *MessageQueue) release() {
	select {
	case <-q.maxConcurrent:
	default:
	}
}
