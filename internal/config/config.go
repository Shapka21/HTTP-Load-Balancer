package config

// структура конфигурации
type Config struct {
	Port     int      `json:"port"`
	Backends []string `json:"backends"`
}
