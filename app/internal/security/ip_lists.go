package security

import (
	"database/sql"
	"status/app/internal/database"
)

// IsWhitelisted checks if an IP is in the whitelist
func IsWhitelisted(ip string) bool {
	var count int
	err := database.DB.QueryRow(`SELECT COUNT(*) FROM ip_whitelist WHERE ip_address = ?`, ip).Scan(&count)
	return err == nil && count > 0
}

// IsBlacklisted checks if an IP is in the blacklist
func IsBlacklisted(ip string) (bool, bool) {
	var permanent int
	err := database.DB.QueryRow(`SELECT permanent FROM ip_blacklist WHERE ip_address = ?`, ip).Scan(&permanent)
	if err != nil {
		return false, false
	}
	return true, permanent == 1
}

// AddToWhitelist adds an IP to the whitelist
func AddToWhitelist(ip, note string) error {
	_, err := database.DB.Exec(`
		INSERT OR REPLACE INTO ip_whitelist (ip_address, note, created_at) 
		VALUES (?, ?, datetime('now'))
	`, ip, note)
	return err
}

// RemoveFromWhitelist removes an IP from the whitelist
func RemoveFromWhitelist(ip string) error {
	_, err := database.DB.Exec(`DELETE FROM ip_whitelist WHERE ip_address = ?`, ip)
	return err
}

// AddToBlacklist adds an IP to the blacklist
func AddToBlacklist(ip, note string, permanent bool) error {
	perm := 0
	if permanent {
		perm = 1
	}
	_, err := database.DB.Exec(`
		INSERT OR REPLACE INTO ip_blacklist (ip_address, permanent, note, created_at) 
		VALUES (?, ?, ?, datetime('now'))
	`, ip, perm, note)
	return err
}

// RemoveFromBlacklist removes an IP from the blacklist
func RemoveFromBlacklist(ip string) error {
	_, err := database.DB.Exec(`DELETE FROM ip_blacklist WHERE ip_address = ?`, ip)
	return err
}

// ListWhitelist returns all whitelisted IPs
func ListWhitelist() ([]map[string]interface{}, error) {
	rows, err := database.DB.Query(`SELECT ip_address, note, created_at FROM ip_whitelist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ip string
		var note sql.NullString
		var createdAt string
		if err := rows.Scan(&ip, &note, &createdAt); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"ip":         ip,
			"note":       note.String,
			"created_at": createdAt,
		})
	}
	return results, nil
}

// ListBlacklist returns all blacklisted IPs
func ListBlacklist() ([]map[string]interface{}, error) {
	rows, err := database.DB.Query(`SELECT ip_address, permanent, note, created_at FROM ip_blacklist ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var ip string
		var permanent int
		var note sql.NullString
		var createdAt string
		if err := rows.Scan(&ip, &permanent, &note, &createdAt); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"ip":         ip,
			"permanent":  permanent == 1,
			"note":       note.String,
			"created_at": createdAt,
		})
	}
	return results, nil
}
