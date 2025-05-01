package main

import (
	conf "HTTP-Load-Balancer/internal/config"
	loadB "HTTP-Load-Balancer/internal/http-server/loadBalancer"
	rl "HTTP-Load-Balancer/internal/http-server/rateLimiting"
	"HTTP-Load-Balancer/internal/lib/getIP"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// Инициализация rate limiter
	limiter := &rl.RateLimiter{
		Buckets: make(map[string]*rl.ClientBucket),
		Stop1:   make(chan struct{}),
	}

	// Добавление стандартных лимитов (пример)
	limiter.AddClient("default", 100, 10) // 100 запросов, 10 в секунду

	handler := &rl.RateLimitHandler{Limiter: limiter}

	configFile := flag.String("config", "config.json", "Path to config file")
	flag.Parse()

	// Чтение конфигурации
	configData, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	var config conf.Config
	err = json.Unmarshal(configData, &config)
	if err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}

	// Создание балансировщика
	lb, err := loadB.NewLoadBalancer(config.Backends)
	if err != nil {
		log.Fatalf("Error creating load balancer: %v", err)
	}

	// Запуск проверки здоровья бэкендов
	go lb.HealthCheck(30 * time.Second)

	// Создаем основной маршрутизатор
	mux := http.NewServeMux()

	// Оборачиваем балансировщик в middleware
	mux.Handle("/", rateLimitMiddleware(lb, limiter))

	// Добавляем другие обработчики
	mux.HandleFunc("/getUser", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.GetClients(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/addUser", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.AddClient(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Настройка сервера
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: mux,
	}

	log.Printf("Load balancer with rate limiting started on port %d", config.Port)
	log.Fatal(server.ListenAndServe())
}

func rateLimitMiddleware(next http.Handler, rateLimit *rl.RateLimiter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		clientID := getIP.GetUserIP(r)

		if rateLimit.Buckets[clientID] == nil {
			rateLimit.AddClient(clientID, 100, 2)
		}

		if !rateLimit.Allow(clientID) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
