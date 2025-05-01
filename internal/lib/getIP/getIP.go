package getIP

import (
	"net"
	"net/http"
	"strings"
)

func GetUserIP(r *http.Request) string {
	// 1. Самый простой способ (но ненадежный за прокси)
	ip := r.RemoteAddr
	if strings.Contains(ip, ":") {
		ip, _, _ = net.SplitHostPort(ip) // Удаляем порт если есть
	}

	// 2. Проверка заголовков, если за прокси
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Берем первый IP из списка (клиентский)
		ips := strings.Split(forwarded, ",")
		ip = strings.TrimSpace(ips[0])
	}

	// 3. Альтернативные прокси-заголовки
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		ip = realIP
	}

	return ip
}
