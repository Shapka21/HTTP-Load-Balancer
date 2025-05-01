package ratelimit

import (
	"HTTP-Load-Balancer/internal/lib/getIP"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ClientBucket хранит состояние токенов для одного клиента
type ClientBucket struct {
	capacity    int
	tokens      int
	rate        float64
	lastUpdated time.Time
	mu          sync.Mutex
}

// RateLimiter управляет ограничениями для всех клиентов
type RateLimiter struct {
	Buckets map[string]*ClientBucket
	mu      sync.RWMutex
	Stop1   chan struct{}
}

type ClientInfo struct {
	ClientID    string  `json:"client_id"`
	Capacity    int     `json:"capacity"`
	Tokens      int     `json:"tokens"`
	Rate        float64 `json:"rate"`
	LastUpdated string  `json:"last_updated"`
}

type RateLimitHandler struct {
	Limiter *RateLimiter
}

func InitRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		Buckets: make(map[string]*ClientBucket),
		Stop1:   make(chan struct{}),
	}

	// Запускаем фоновое пополнение токенов
	go rl.refillBuckets()

	return rl
}

func (rl *RateLimiter) Stop() {
	close(rl.Stop1)
}

func (rl *RateLimiter) AddClient(clientID string, capacity int, rate float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.Buckets[clientID] = &ClientBucket{
		capacity:    capacity,
		tokens:      capacity,
		rate:        rate,
		lastUpdated: time.Now(),
	}
}

// Allow проверяет, разрешен ли запрос для клиента
func (rl *RateLimiter) Allow(clientID string) bool {
	rl.mu.RLock()
	bucket, exists := rl.Buckets[clientID]
	rl.mu.RUnlock()

	if !exists {
		return true
	}

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// Пополняем токены перед проверкой
	rl.refill(bucket)

	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}
	return false
}

// refill пополняет токены в bucket с учетом прошедшего времени
func (rl *RateLimiter) refill(bucket *ClientBucket) {
	now := time.Now()
	elapsed := now.Sub(bucket.lastUpdated).Seconds()
	tokensToAdd := int(elapsed * bucket.rate)

	if tokensToAdd > 0 {
		bucket.tokens = min(bucket.tokens+tokensToAdd, bucket.capacity)
		bucket.lastUpdated = now
	}
}

// refillBuckets периодически очищает старые bucket'ы
func (rl *RateLimiter) refillBuckets() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanupOldBuckets()
		case <-rl.Stop1:
			return
		}
	}
}

func (rl *RateLimiter) cleanupOldBuckets() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	threshold := time.Now().Add(-24 * time.Hour)

	for client, bucket := range rl.Buckets {
		bucket.mu.Lock()
		if bucket.lastUpdated.Before(threshold) {
			delete(rl.Buckets, client)
		}
		bucket.mu.Unlock()
	}
}

func (h *RateLimitHandler) GetClients(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.Limiter.mu.RLock()
	defer h.Limiter.mu.RUnlock()

	clients := make([]ClientInfo, 0, len(h.Limiter.Buckets))
	for clientID, bucket := range h.Limiter.Buckets {
		bucket.mu.Lock()
		clients = append(clients, ClientInfo{
			ClientID:    clientID,
			Capacity:    bucket.capacity,
			Tokens:      bucket.tokens,
			Rate:        bucket.rate,
			LastUpdated: bucket.lastUpdated.Format(time.RFC3339),
		})
		bucket.mu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clients)
}

func (h *RateLimitHandler) AddClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Capacity int     `json:"capacity"`
		Rate     float64 `json:"rate"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ClientID := getIP.GetUserIP(r)

	if req.Capacity <= 0 {
		http.Error(w, "capacity must be positive", http.StatusBadRequest)
		return
	}

	if req.Rate <= 0 {
		http.Error(w, "rate must be positive", http.StatusBadRequest)
		return
	}

	h.Limiter.AddClient(ClientID, req.Capacity, req.Rate)
}
