package broker

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrStreamNotFound     = errors.New("stream not found")
	ErrSubscriberNotFound = errors.New("subscriber not found")
	ErrNoMessages         = errors.New("no messages")
	ErrAckInvalid         = errors.New("invalid ack")
)

type Broker struct {
	mu            sync.RWMutex
	streams       map[string]*stream
	store         *FileStore
	ackTimeout    time.Duration
	maxDeliveries int
	stopCh        chan struct{}
	metrics       Metrics
}

type stream struct {
	Name        string                     `json:"name"`
	Type        StreamType                 `json:"type"`
	QueueMode   QueueMode                  `json:"queue_mode"`
	Messages    []Message                  `json:"messages"`
	Subscribers map[string]*SubscriberState `json:"subscribers"`
	Available   []string                   `json:"available"`
	InFlight    map[string]*Delivery       `json:"in_flight"`
	DLQ         []string                   `json:"dlq"`

	msgIndex map[string]int `json:"-"`
}

func (s *stream) rebuildIndexes() {
	s.msgIndex = make(map[string]int, len(s.Messages))
	for i := range s.Messages {
		s.msgIndex[s.Messages[i].ID] = i
	}
	if s.Subscribers == nil {
		s.Subscribers = map[string]*SubscriberState{}
	}
	if s.InFlight == nil {
		s.InFlight = map[string]*Delivery{}
	}
}

func New(dataDir string, ackTimeout time.Duration, maxDeliveries int) (*Broker, error) {
	store, err := NewFileStore(dataDir)
	if err != nil {
		return nil, err
	}
	b := &Broker{
		streams:       map[string]*stream{},
		store:         store,
		ackTimeout:    ackTimeout,
		maxDeliveries: maxDeliveries,
		stopCh:        make(chan struct{}),
	}
	if err := b.load(); err != nil {
		return nil, err
	}
	go b.requeueWorker()
	return b, nil
}

func (b *Broker) Close() {
	close(b.stopCh)
}

func (b *Broker) load() error {
	stored, err := b.store.LoadStreams()
	if err != nil {
		return err
	}
	for _, st := range stored {
		b.streams[st.Name] = st
	}
	return nil
}

func (b *Broker) CreateStream(name string, stype StreamType, mode QueueMode) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.streams[name]; ok {
		return nil
	}
	if stype == StreamTypeTopic {
		mode = QueueModeFIFO
	}
	st := &stream{
		Name:        name,
		Type:        stype,
		QueueMode:   mode,
		Messages:    []Message{},
		Subscribers: map[string]*SubscriberState{},
		Available:   []string{},
		InFlight:    map[string]*Delivery{},
		DLQ:         []string{},
		msgIndex:    map[string]int{},
	}
	b.streams[name] = st
	return b.store.SaveStream(st)
}

func (b *Broker) Subscribe(streamName, subscriber string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	st, ok := b.streams[streamName]
	if !ok {
		return ErrStreamNotFound
	}
	if _, ok := st.Subscribers[subscriber]; !ok {
		st.Subscribers[subscriber] = &SubscriberState{
			Subscriber: subscriber,
			Offset:     0,
		}
		atomic.AddInt64(&b.metrics.ActiveSubscribers, 1)
	}
	return b.store.SaveStream(st)
}

func (b *Broker) Publish(streamName, payload string, priority int, ttl time.Duration) (Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	st, ok := b.streams[streamName]
	if !ok {
		return Message{}, ErrStreamNotFound
	}
	msg := Message{
		ID:        strconv.FormatInt(time.Now().UnixNano(), 10),
		Payload:   payload,
		Priority:  priority,
		CreatedAt: time.Now().UTC(),
	}
	if ttl > 0 {
		msg.ExpiresAt = msg.CreatedAt.Add(ttl)
	}

	st.Messages = append(st.Messages, msg)
	st.msgIndex[msg.ID] = len(st.Messages) - 1

	if st.Type == StreamTypeQueue {
		b.pushAvailable(st, msg.ID, msg.Priority)
	}
	atomic.AddInt64(&b.metrics.Published, 1)

	return msg, b.store.SaveStream(st)
}

func (b *Broker) Pull(streamName, subscriber string, batch int) ([]Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if batch <= 0 {
		batch = 1
	}
	st, ok := b.streams[streamName]
	if !ok {
		return nil, ErrStreamNotFound
	}
	sub, ok := st.Subscribers[subscriber]
	if !ok {
		return nil, ErrSubscriberNotFound
	}

	var out []Message
	if st.Type == StreamTypeTopic {
		for len(out) < batch && sub.Offset < len(st.Messages) {
			msg := st.Messages[sub.Offset]
			if b.isExpired(msg) {
				st.DLQ = append(st.DLQ, msg.ID)
				sub.Offset++
				atomic.AddInt64(&b.metrics.ExpiredToDLQ, 1)
				continue
			}
			out = append(out, msg)
			break
		}
		if len(out) == 0 {
			return nil, ErrNoMessages
		}
		atomic.AddInt64(&b.metrics.Delivered, int64(len(out)))
		return out, b.store.SaveStream(st)
	}

	for len(out) < batch {
		msgID, ok := b.popAvailable(st)
		if !ok {
			break
		}
		idx := st.msgIndex[msgID]
		msg := st.Messages[idx]
		if b.isExpired(msg) {
			st.DLQ = append(st.DLQ, msg.ID)
			atomic.AddInt64(&b.metrics.ExpiredToDLQ, 1)
			continue
		}
		d := &Delivery{
			MessageID:   msg.ID,
			Subscriber:  subscriber,
			Attempts:    1,
			DeliveredAt: time.Now().UTC(),
			AckDeadline: time.Now().UTC().Add(b.ackTimeout),
		}
		if old, ok := st.InFlight[msg.ID]; ok {
			d.Attempts = old.Attempts + 1
		}
		st.InFlight[msg.ID] = d
		out = append(out, msg)
	}

	if len(out) == 0 {
		return nil, ErrNoMessages
	}
	atomic.AddInt64(&b.metrics.Delivered, int64(len(out)))
	return out, b.store.SaveStream(st)
}

func (b *Broker) Ack(streamName, subscriber, messageID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	st, ok := b.streams[streamName]
	if !ok {
		return ErrStreamNotFound
	}
	if _, ok := st.Subscribers[subscriber]; !ok {
		return ErrSubscriberNotFound
	}

	if st.Type == StreamTypeTopic {
		sub := st.Subscribers[subscriber]
		if sub.Offset >= len(st.Messages) {
			return ErrAckInvalid
		}
		current := st.Messages[sub.Offset]
		if current.ID != messageID {
			return ErrAckInvalid
		}
		sub.Offset++
		atomic.AddInt64(&b.metrics.Acked, 1)
		return b.store.SaveStream(st)
	}

	d, ok := st.InFlight[messageID]
	if !ok || d.Subscriber != subscriber {
		return ErrAckInvalid
	}
	delete(st.InFlight, messageID)
	atomic.AddInt64(&b.metrics.Acked, 1)
	return b.store.SaveStream(st)
}

func (b *Broker) Metrics() Metrics {
	return Metrics{
		Published:         atomic.LoadInt64(&b.metrics.Published),
		Delivered:         atomic.LoadInt64(&b.metrics.Delivered),
		Acked:             atomic.LoadInt64(&b.metrics.Acked),
		Redelivered:       atomic.LoadInt64(&b.metrics.Redelivered),
		ExpiredToDLQ:      atomic.LoadInt64(&b.metrics.ExpiredToDLQ),
		MaxDeliveryToDLQ:  atomic.LoadInt64(&b.metrics.MaxDeliveryToDLQ),
		ActiveSubscribers: atomic.LoadInt64(&b.metrics.ActiveSubscribers),
	}
}

func (b *Broker) requeueWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.requeueExpiredAcks()
		case <-b.stopCh:
			return
		}
	}
}

func (b *Broker) requeueExpiredAcks() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now().UTC()
	for _, st := range b.streams {
		if st.Type != StreamTypeQueue {
			continue
		}
		changed := false
		for msgID, d := range st.InFlight {
			if now.Before(d.AckDeadline) {
				continue
			}
			if d.Attempts >= b.maxDeliveries {
				delete(st.InFlight, msgID)
				st.DLQ = append(st.DLQ, msgID)
				atomic.AddInt64(&b.metrics.MaxDeliveryToDLQ, 1)
				changed = true
				continue
			}
			delete(st.InFlight, msgID)
			d.Attempts++
			st.InFlight[msgID] = d
			b.pushAvailable(st, msgID, st.Messages[st.msgIndex[msgID]].Priority)
			atomic.AddInt64(&b.metrics.Redelivered, 1)
			changed = true
		}
		if changed {
			_ = b.store.SaveStream(st)
		}
	}
}

func (b *Broker) isExpired(msg Message) bool {
	return !msg.ExpiresAt.IsZero() && time.Now().UTC().After(msg.ExpiresAt)
}

func (b *Broker) pushAvailable(st *stream, msgID string, priority int) {
	st.Available = append(st.Available, msgID)
	slices.SortStableFunc(st.Available, func(a, b string) int {
		pa := st.Messages[st.msgIndex[a]].Priority
		pb := st.Messages[st.msgIndex[b]].Priority
		if pa != pb {
			return pb - pa
		}
		ca := st.Messages[st.msgIndex[a]].CreatedAt
		cb := st.Messages[st.msgIndex[b]].CreatedAt
		if st.QueueMode == QueueModeLIFO {
			if ca.After(cb) {
				return -1
			}
			if ca.Before(cb) {
				return 1
			}
			return 0
		}
		if ca.Before(cb) {
			return -1
		}
		if ca.After(cb) {
			return 1
		}
		return 0
	})
}

func (b *Broker) popAvailable(st *stream) (string, bool) {
	if len(st.Available) == 0 {
		return "", false
	}
	id := st.Available[0]
	st.Available = st.Available[1:]
	return id, true
}

func ParseStreamType(v string) (StreamType, error) {
	switch StreamType(v) {
	case StreamTypeTopic, StreamTypeQueue:
		return StreamType(v), nil
	default:
		return "", fmt.Errorf("unknown stream type: %s", v)
	}
}

func ParseQueueMode(v string) (QueueMode, error) {
	switch QueueMode(v) {
	case QueueModeFIFO, QueueModeLIFO:
		return QueueMode(v), nil
	default:
		return "", fmt.Errorf("unknown queue mode: %s", v)
	}
}
