package loadbalancer

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// структкра, содержащая состояние балансировщика
type LoadBalancer struct {
	backends []*url.URL
	proxy    *httputil.ReverseProxy
	current  uint64
	mutex    sync.Mutex
}

// функция для определения, на какой бэкенд направить запрос
func (lb *LoadBalancer) director(req *http.Request) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	target := lb.backends[lb.current%uint64(len(lb.backends))]
	lb.current++

	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.URL.Path = target.Path + req.URL.Path
	req.Host = target.Host

	log.Printf("Forwarding request to %s", target.String())
}

// modifyResponse может модифицировать ответ от бэкенда
func (lb *LoadBalancer) modifyResponse(resp *http.Response) error {
	resp.Header.Set("X-LoadBalancer", "GoLB/1.0")

	if target := resp.Request.URL; target != nil {
		resp.Header.Set("X-Backend-Server", target.Host)
	}

	log.Printf("Backend %s returned status: %d", resp.Request.URL.Host, resp.StatusCode)

	return nil
}

// Обработчик ошибок при обращении к бэкендам
func (lb *LoadBalancer) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("Error connecting to backend: %v", err)
	w.WriteHeader(http.StatusBadGateway)
	w.Write([]byte("502 Bad Gateway"))
}

// создание нового экземпляра балансировщика
func NewLoadBalancer(backendUrls []string) (*LoadBalancer, error) {
	lb := &LoadBalancer{}
	for _, backend := range backendUrls {
		u, err := url.Parse(backend)
		if err != nil {
			return nil, err
		}
		lb.backends = append(lb.backends, u)
	}

	lb.proxy = &httputil.ReverseProxy{
		Director:       lb.director,
		ModifyResponse: lb.modifyResponse,
		ErrorHandler:   lb.errorHandler,
	}

	return lb, nil
}

// реализауция интерфейса http.Handler
func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(lb.backends) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("503 Service Unavailable - no backends available"))
		return
	}

	lb.proxy.ServeHTTP(w, r)
}

func (lb *LoadBalancer) HealthCheck(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		var healthyBackends []*url.URL

		for _, backend := range lb.backends {
			resp, err := http.Get(backend.String())
			if err == nil && resp.StatusCode < 500 {
				healthyBackends = append(healthyBackends, backend)
				resp.Body.Close()
			} else {
				log.Printf("Backend %s is unhealthy", backend.String())
			}
		}

		lb.mutex.Lock()
		lb.backends = healthyBackends
		if lb.current >= uint64(len(lb.backends)) {
			lb.current = 0
		}
		lb.mutex.Unlock()
	}
}
