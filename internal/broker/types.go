package broker

import "time"

type StreamType string

const (
	StreamTypeTopic StreamType = "topic"
	StreamTypeQueue StreamType = "queue"
)

type QueueMode string

const (
	QueueModeFIFO QueueMode = "fifo"
	QueueModeLIFO QueueMode = "lifo"
)

type Message struct {
	ID        string    `json:"id"`
	Payload   string    `json:"payload"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type Delivery struct {
	MessageID    string    `json:"message_id"`
	Subscriber   string    `json:"subscriber"`
	Attempts     int       `json:"attempts"`
	DeliveredAt  time.Time `json:"delivered_at"`
	AckDeadline  time.Time `json:"ack_deadline"`
	LastErrorMsg string    `json:"last_error_msg,omitempty"`
}

type SubscriberState struct {
	Subscriber string `json:"subscriber"`
	Offset     int    `json:"offset"`
}

type Metrics struct {
	Published         int64 `json:"published"`
	Delivered         int64 `json:"delivered"`
	Acked             int64 `json:"acked"`
	Redelivered       int64 `json:"redelivered"`
	ExpiredToDLQ      int64 `json:"expired_to_dlq"`
	MaxDeliveryToDLQ  int64 `json:"max_delivery_to_dlq"`
	ActiveSubscribers int64 `json:"active_subscribers"`
}
