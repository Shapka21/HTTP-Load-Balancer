package getIP

import (
	"net"
	"net/http"
	"strings"
)

func GetUserIP(r *http.Request) string {
	ip := r.RemoteAddr
	if strings.Contains(ip, ":") {
		ip, _, _ = net.SplitHostPort(ip)
	}

	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Берем первый IP из списка (клиентский)
		ips := strings.Split(forwarded, ",")
		ip = strings.TrimSpace(ips[0])
	}

	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		ip = realIP
	}

	return ip
}
