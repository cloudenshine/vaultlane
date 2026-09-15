package delivery

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"faka-gateway/internal/store"
)

// Task 履约任务
type Task struct {
	OrderID int64
	TradeNo string
	Retry   int
}

// Queue 异步履约发货队列
type Queue struct {
	store      *store.Store
	dispatcher *Dispatcher
	logger     *slog.Logger
	tasks      chan Task
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewQueue 构造异步履约队列
func NewQueue(st *store.Store, d *Dispatcher, workers int, bufferSize int, logger *slog.Logger) *Queue {
	if logger == nil {
		logger = slog.Default()
	}
	if workers <= 0 {
		workers = 4
	}
	if bufferSize <= 0 {
		bufferSize = 256
	}
	ctx, cancel := context.WithCancel(context.Background())
	q := &Queue{
		store:      st,
		dispatcher: d,
		logger:     logger,
		tasks:      make(chan Task, bufferSize),
		ctx:        ctx,
		cancel:     cancel,
	}

	// 启动 Worker
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(i + 1)
	}
	return q
}

// Push 将已支付订单推入发货队列（非阻塞）
func (q *Queue) Push(orderID int64, tradeNo string) bool {
	select {
	case q.tasks <- Task{OrderID: orderID, TradeNo: tradeNo, Retry: 0}:
		return true
	default:
		q.logger.Warn("delivery queue is full, will execute in background", "order_id", orderID, "trade_no", tradeNo)
		go q.process(Task{OrderID: orderID, TradeNo: tradeNo, Retry: 0})
		return true
	}
}

func (q *Queue) worker(workerID int) {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case task, ok := <-q.tasks:
			if !ok {
				return
			}
			q.process(task)
		}
	}
}

func (q *Queue) process(t Task) {
	var o *store.Order
	var err error
	if t.OrderID > 0 {
		o, err = q.store.GetOrderByID(t.OrderID)
	} else if t.TradeNo != "" {
		o, err = q.store.GetOrderByTradeNo(t.TradeNo)
	}
	if err != nil || o == nil {
		q.logger.Error("queue: fetch order failed", "order_id", t.OrderID, "trade_no", t.TradeNo, "err", err)
		return
	}

	// 已发货或退款状态直接跳过
	if o.Status >= 2 {
		return
	}

	// 标记待发货
	if o.Status == 0 {
		o.Status = 1
		_ = q.store.UpdateOrder(o)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := q.dispatcher.Dispatch(ctx, o); err != nil {
		q.logger.Warn("delivery failed", "order_id", o.ID, "trade_no", o.TradeNo, "retry", t.Retry, "err", err)
		o.UpstreamMsg = err.Error()
		_ = q.store.UpdateOrder(o)

		// 指数退避重试 (最大重试 2 次)
		if t.Retry < 2 {
			backoff := time.Duration(1<<t.Retry) * 2 * time.Second
			time.AfterFunc(backoff, func() {
				q.Push(t.OrderID, t.TradeNo)
			})
		}
		return
	}

	// 履约成功更新订单
	o.Status = 2
	if err := q.store.UpdateOrder(o); err != nil {
		q.logger.Error("queue: update order success status failed", "order_id", o.ID, "err", err)
	} else {
		q.logger.Info("queue: order delivered successfully", "order_id", o.ID, "trade_no", o.TradeNo, "source", o.Source)
	}
}

// Stop 优雅关闭队列
func (q *Queue) Stop() {
	q.cancel()
	close(q.tasks)
	q.wg.Wait()
}
