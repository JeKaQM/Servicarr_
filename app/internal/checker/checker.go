package checker

import (
	"log"
	"net"
	"net/http"
	"status/app/internal/models"
	"strings"
	"time"
)

// HTTPCheck performs a health check on a service
func HTTPCheck(url string, timeout time.Duration, minOK, maxOK int) (ok bool, code int, ms *int, errStr string) {
	if strings.HasPrefix(url, "tcp://") {
		addr := strings.TrimPrefix(url, "tcp://")
		t0 := time.Now()
		conn, err := net.DialTimeout("tcp", addr, timeout)
		d := int(time.Since(t0).Milliseconds())
		ms = &d
		if err != nil {
			log.Printf("tcp check error addr=%s err=%v", addr, err)
			return false, 0, nil, err.Error()
		}
		_ = conn.Close()
		return true, 0, ms, ""
	}

	client := &http.Client{Timeout: timeout}
	t0 := time.Now()
	resp, err := client.Get(url)
	d := int(time.Since(t0).Milliseconds())
	ms = &d
	if err != nil {
		log.Printf("http check error url=%s err=%v", url, err)
		return false, 0, nil, err.Error()
	}
	defer resp.Body.Close()
	ok = resp.StatusCode >= minOK && resp.StatusCode <= maxOK
	return ok, resp.StatusCode, ms, ""
}

// FindServiceByKey finds a service in the slice by its key
func FindServiceByKey(services []*models.Service, key string) *models.Service {
	for _, s := range services {
		if s.Key == key {
			return s
		}
	}
	return nil
}
