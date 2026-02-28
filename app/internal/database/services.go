package database

import (
	"log"
	"status/app/internal/crypto"
	"status/app/internal/models"
	"time"
)

// InsertSample records a service check sample
func InsertSample(ts time.Time, key string, ok bool, status int, ms *int) {
	okInt := 0
	if ok {
		okInt = 1
	}
	var msVal any
	if ms != nil {
		msVal = *ms
	}

	_, _ = DB.Exec(`INSERT INTO samples (taken_at,service_key,ok,http_status,latency_ms)
		VALUES (?,?,?,?,?)`,
		ts.UTC().Format(time.RFC3339), key, okInt, status, msVal)
}

// GetServiceDisabledState loads service disabled state from database
func GetServiceDisabledState(key string) (bool, error) {
	var disabled int
	err := DB.QueryRow(`SELECT disabled FROM service_state WHERE service_key = ?`, key).Scan(&disabled)
	if err != nil {
		// sql.ErrNoRows means not disabled
		return false, nil
	}
	return disabled != 0, nil
}

// SetServiceDisabledState updates service disabled state in database
func SetServiceDisabledState(key string, disabled bool) error {
	disabledInt := 0
	if disabled {
		disabledInt = 1
	}
	_, err := DB.Exec(`
		INSERT INTO service_state (service_key, disabled, updated_at) 
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(service_key) DO UPDATE SET disabled=?, updated_at=datetime('now')`,
		key, disabledInt, disabledInt)
	return err
}

// GetAllServices returns all services from the database ordered by display_order
func GetAllServices() ([]models.ServiceConfig, error) {
	rows, err := DB.Query(`
		SELECT id, key, name, url, service_type, COALESCE(icon, ''), COALESCE(icon_url, ''), COALESCE(api_token, ''),
		       display_order, visible, check_type, check_interval, timeout, expected_min, expected_max,
		       COALESCE(depends_on, ''), COALESCE(connected_to, ''), created_at, COALESCE(updated_at, '')
		FROM services ORDER BY display_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []models.ServiceConfig
	for rows.Next() {
		var s models.ServiceConfig
		var visible int
		err := rows.Scan(&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
			&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
			&s.ExpectedMin, &s.ExpectedMax, &s.DependsOn, &s.ConnectedTo, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		s.Visible = visible != 0
		decryptServiceToken(&s)
		services = append(services, s)
	}
	return services, nil
}

// GetVisibleServices returns only visible services from the database
func GetVisibleServices() ([]models.ServiceConfig, error) {
	rows, err := DB.Query(`
		SELECT id, key, name, url, service_type, COALESCE(icon, ''), COALESCE(icon_url, ''), COALESCE(api_token, ''),
		       display_order, visible, check_type, check_interval, timeout, expected_min, expected_max,
		       COALESCE(depends_on, ''), COALESCE(connected_to, ''), created_at, COALESCE(updated_at, '')
		FROM services WHERE visible = 1 ORDER BY display_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []models.ServiceConfig
	for rows.Next() {
		var s models.ServiceConfig
		var visible int
		err := rows.Scan(&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
			&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
			&s.ExpectedMin, &s.ExpectedMax, &s.DependsOn, &s.ConnectedTo, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		s.Visible = visible != 0
		decryptServiceToken(&s)
		services = append(services, s)
	}
	return services, nil
}

// GetServiceByID returns a service by its ID
func GetServiceByID(id int) (*models.ServiceConfig, error) {
	var s models.ServiceConfig
	var visible int
	err := DB.QueryRow(`
		SELECT id, key, name, url, service_type, COALESCE(icon, ''), COALESCE(icon_url, ''), COALESCE(api_token, ''),
		       display_order, visible, check_type, check_interval, timeout, expected_min, expected_max,
		       COALESCE(depends_on, ''), COALESCE(connected_to, ''), created_at, COALESCE(updated_at, '')
		FROM services WHERE id = ?`, id).Scan(
		&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
		&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
		&s.ExpectedMin, &s.ExpectedMax, &s.DependsOn, &s.ConnectedTo, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Visible = visible != 0
	decryptServiceToken(&s)
	return &s, nil
}

// GetServiceByKey returns a service by its key
func GetServiceByKey(key string) (*models.ServiceConfig, error) {
	var s models.ServiceConfig
	var visible int
	err := DB.QueryRow(`
		SELECT id, key, name, url, service_type, COALESCE(icon, ''), COALESCE(icon_url, ''), COALESCE(api_token, ''),
		       display_order, visible, check_type, check_interval, timeout, expected_min, expected_max,
		       COALESCE(depends_on, ''), COALESCE(connected_to, ''), created_at, COALESCE(updated_at, '')
		FROM services WHERE key = ?`, key).Scan(
		&s.ID, &s.Key, &s.Name, &s.URL, &s.ServiceType, &s.Icon, &s.IconURL, &s.APIToken,
		&s.DisplayOrder, &visible, &s.CheckType, &s.CheckInterval, &s.Timeout,
		&s.ExpectedMin, &s.ExpectedMax, &s.DependsOn, &s.ConnectedTo, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Visible = visible != 0
	decryptServiceToken(&s)
	return &s, nil
}

// decryptServiceToken decrypts api_token in-place on a ServiceConfig.
func decryptServiceToken(s *models.ServiceConfig) {
	if s.APIToken != "" {
		plain, err := crypto.Decrypt(s.APIToken)
		if err != nil {
			log.Printf("Warning: failed to decrypt token for service %s: %v", s.Key, err)
			// Keep the raw value — may be legacy plaintext
		} else {
			s.APIToken = plain
		}
	}
}

// CreateService inserts a new service into the database
func CreateService(s *models.ServiceConfig) (int64, error) {
	visible := 0
	if s.Visible {
		visible = 1
	}

	// Auto-assign display order only when not explicitly provided
	if s.DisplayOrder < 0 {
		var maxOrder int
		_ = DB.QueryRow(`SELECT COALESCE(MAX(display_order), -1) FROM services`).Scan(&maxOrder)
		s.DisplayOrder = maxOrder + 1
	}

	// Encrypt API token before storing
	encToken, err := crypto.Encrypt(s.APIToken)
	if err != nil {
		log.Printf("Warning: failed to encrypt token for service %s: %v", s.Key, err)
		encToken = s.APIToken // fallback to plaintext if encryption fails
	}

	result, err := DB.Exec(`
		INSERT INTO services (key, name, url, service_type, icon, icon_url, api_token, display_order, visible,
		                      check_type, check_interval, timeout, expected_min, expected_max, depends_on, connected_to, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))`,
		s.Key, s.Name, s.URL, s.ServiceType, s.Icon, s.IconURL, encToken, s.DisplayOrder, visible,
		s.CheckType, s.CheckInterval, s.Timeout, s.ExpectedMin, s.ExpectedMax, s.DependsOn, s.ConnectedTo)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateService updates an existing service
func UpdateService(s *models.ServiceConfig) error {
	visible := 0
	if s.Visible {
		visible = 1
	}

	// Encrypt API token before storing
	encToken, err := crypto.Encrypt(s.APIToken)
	if err != nil {
		log.Printf("Warning: failed to encrypt token for service %d: %v", s.ID, err)
		encToken = s.APIToken
	}

	_, err = DB.Exec(`
		UPDATE services SET name=?, url=?, service_type=?, icon=?, icon_url=?, api_token=?, display_order=?,
		                    visible=?, check_type=?, check_interval=?, timeout=?, expected_min=?,
		                    expected_max=?, depends_on=?, connected_to=?, updated_at=datetime('now')
		WHERE id = ?`,
		s.Name, s.URL, s.ServiceType, s.Icon, s.IconURL, encToken, s.DisplayOrder, visible,
		s.CheckType, s.CheckInterval, s.Timeout, s.ExpectedMin, s.ExpectedMax, s.DependsOn, s.ConnectedTo, s.ID)
	return err
}

// DeleteService removes a service from the database
func DeleteService(id int) error {
	_, err := DB.Exec(`DELETE FROM services WHERE id = ?`, id)
	return err
}

// UpdateServiceVisibility toggles service visibility
func UpdateServiceVisibility(id int, visible bool) error {
	v := 0
	if visible {
		v = 1
	}
	_, err := DB.Exec(`UPDATE services SET visible = ?, updated_at = datetime('now') WHERE id = ?`, v, id)
	return err
}

// UpdateServiceOrder updates the display order of services
func UpdateServiceOrder(orders map[int]int) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE services SET display_order = ?, updated_at = datetime('now') WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for id, order := range orders {
		_, err := stmt.Exec(order, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetServiceCount returns the number of services
func GetServiceCount() (int, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM services`).Scan(&count)
	return count, err
}
