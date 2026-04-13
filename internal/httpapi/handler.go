package httpapi

import (
	"brocker/internal/broker"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type Handler struct {
	b *broker.Broker
}

func NewHandler(b *broker.Broker) *Handler {
	return &Handler{b: b}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/streams", h.createStream)
	mux.HandleFunc("/subscribe", h.subscribe)
	mux.HandleFunc("/publish", h.publish)
	mux.HandleFunc("/pull", h.pull)
	mux.HandleFunc("/ack", h.ack)
	mux.HandleFunc("/metrics", h.metrics)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createStreamRequest struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	QueueMode string `json:"queue_mode"`
}

func (h *Handler) createStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req createStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	stype, err := broker.ParseStreamType(req.Type)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	mode := broker.QueueModeFIFO
	if req.QueueMode != "" {
		mode, err = broker.ParseQueueMode(req.QueueMode)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.b.CreateStream(req.Name, stype, mode); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

type subscribeRequest struct {
	Stream     string `json:"stream"`
	Subscriber string `json:"subscriber"`
}

func (h *Handler) subscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.b.Subscribe(req.Stream, req.Subscriber); err != nil {
		mapBrokerErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "subscribed"})
}

type publishRequest struct {
	Stream     string `json:"stream"`
	Payload    string `json:"payload"`
	Priority   int    `json:"priority"`
	TTLSeconds int    `json:"ttl_seconds"`
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	msg, err := h.b.Publish(req.Stream, req.Payload, req.Priority, time.Duration(req.TTLSeconds)*time.Second)
	if err != nil {
		mapBrokerErr(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, msg)
}

type pullRequest struct {
	Stream     string `json:"stream"`
	Subscriber string `json:"subscriber"`
	Batch      int    `json:"batch"`
}

func (h *Handler) pull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req pullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	msgs, err := h.b.Pull(req.Stream, req.Subscriber, req.Batch)
	if err != nil {
		if errors.Is(err, broker.ErrNoMessages) {
			respondJSON(w, http.StatusOK, map[string]any{"messages": []broker.Message{}})
			return
		}
		mapBrokerErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

type ackRequest struct {
	Stream     string `json:"stream"`
	Subscriber string `json:"subscriber"`
	MessageID  string `json:"message_id"`
}

func (h *Handler) ack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req ackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.b.Ack(req.Stream, req.Subscriber, req.MessageID); err != nil {
		mapBrokerErr(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "acked"})
}

func (h *Handler) metrics(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, h.b.Metrics())
}

func mapBrokerErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, broker.ErrStreamNotFound), errors.Is(err, broker.ErrSubscriberNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, broker.ErrAckInvalid):
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		respondError(w, http.StatusInternalServerError, err.Error())
	}
}

func respondJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, code int, msg string) {
	respondJSON(w, code, map[string]string{"error": msg})
}
