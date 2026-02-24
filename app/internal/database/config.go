package database

import (
	"database/sql"
	"status/app/internal/models"
)

// LoadAlertConfig loads email alert configuration from database
func LoadAlertConfig() (*models.AlertConfig, error) {
	var config models.AlertConfig
	err := DB.QueryRow(`SELECT enabled, smtp_host, smtp_port, smtp_user, smtp_password, alert_email, from_email,
		COALESCE(status_page_url, ''), COALESCE(smtp_skip_verify, 0), alert_on_down, alert_on_degraded, alert_on_up,
		COALESCE(discord_webhook_url, ''), COALESCE(discord_enabled, 0),
		COALESCE(telegram_bot_token, ''), COALESCE(telegram_chat_id, ''), COALESCE(telegram_enabled, 0),
		COALESCE(webhook_url, ''), COALESCE(webhook_secret, ''), COALESCE(webhook_enabled, 0)
		FROM alert_config WHERE id = 1`).Scan(
		&config.Enabled, &config.SMTPHost, &config.SMTPPort, &config.SMTPUser,
		&config.SMTPPassword, &config.AlertEmail, &config.FromEmail, &config.StatusPageURL, &config.SMTPSkipVerify,
		&config.AlertOnDown, &config.AlertOnDegraded, &config.AlertOnUp,
		&config.DiscordWebhookURL, &config.DiscordEnabled,
		&config.TelegramBotToken, &config.TelegramChatID, &config.TelegramEnabled,
		&config.WebhookURL, &config.WebhookSecret, &config.WebhookEnabled)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// SaveAlertConfig saves email alert configuration to database
func SaveAlertConfig(config *models.AlertConfig) error {
	_, err := DB.Exec(`INSERT INTO alert_config (id, enabled, smtp_host, smtp_port, smtp_user, smtp_password, alert_email, from_email, status_page_url, smtp_skip_verify,
		alert_on_down, alert_on_degraded, alert_on_up,
		discord_webhook_url, discord_enabled,
		telegram_bot_token, telegram_chat_id, telegram_enabled,
		webhook_url, webhook_secret, webhook_enabled, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET 
			enabled=?, smtp_host=?, smtp_port=?, smtp_user=?, smtp_password=?, alert_email=?, from_email=?, status_page_url=?, smtp_skip_verify=?,
			alert_on_down=?, alert_on_degraded=?, alert_on_up=?,
			discord_webhook_url=?, discord_enabled=?,
			telegram_bot_token=?, telegram_chat_id=?, telegram_enabled=?,
			webhook_url=?, webhook_secret=?, webhook_enabled=?, updated_at=datetime('now')`,
		config.Enabled, config.SMTPHost, config.SMTPPort, config.SMTPUser, config.SMTPPassword,
		config.AlertEmail, config.FromEmail, config.StatusPageURL, config.SMTPSkipVerify,
		config.AlertOnDown, config.AlertOnDegraded, config.AlertOnUp,
		config.DiscordWebhookURL, config.DiscordEnabled,
		config.TelegramBotToken, config.TelegramChatID, config.TelegramEnabled,
		config.WebhookURL, config.WebhookSecret, config.WebhookEnabled,
		config.Enabled, config.SMTPHost, config.SMTPPort, config.SMTPUser, config.SMTPPassword,
		config.AlertEmail, config.FromEmail, config.StatusPageURL, config.SMTPSkipVerify,
		config.AlertOnDown, config.AlertOnDegraded, config.AlertOnUp,
		config.DiscordWebhookURL, config.DiscordEnabled,
		config.TelegramBotToken, config.TelegramChatID, config.TelegramEnabled,
		config.WebhookURL, config.WebhookSecret, config.WebhookEnabled)
	return err
}

// LoadResourcesUIConfig loads resources UI configuration from database
func LoadResourcesUIConfig() (*models.ResourcesUIConfig, error) {
	var config models.ResourcesUIConfig
	var glancesURL sql.NullString
	err := DB.QueryRow(`SELECT enabled, COALESCE(glances_url, ''), cpu, memory, network, temp, storage,
		COALESCE(swap, 0), COALESCE(load, 0), COALESCE(gpu, 0), COALESCE(containers, 0), COALESCE(processes, 0), COALESCE(uptime, 0)
		FROM resources_ui_config WHERE id = 1`).Scan(
		&config.Enabled, &glancesURL, &config.CPU, &config.Memory, &config.Network, &config.Temp, &config.Storage,
		&config.Swap, &config.Load, &config.GPU, &config.Containers, &config.Processes, &config.Uptime,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	config.GlancesURL = glancesURL.String
	return &config, nil
}

// SaveResourcesUIConfig saves resources UI configuration to database
func SaveResourcesUIConfig(config *models.ResourcesUIConfig) error {
	_, err := DB.Exec(`INSERT INTO resources_ui_config (id, enabled, glances_url, cpu, memory, network, temp, storage, swap, load, gpu, containers, processes, uptime, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			enabled=?, glances_url=?, cpu=?, memory=?, network=?, temp=?, storage=?, swap=?, load=?, gpu=?, containers=?, processes=?, uptime=?, updated_at=datetime('now')`,
		config.Enabled, config.GlancesURL, config.CPU, config.Memory, config.Network, config.Temp, config.Storage,
		config.Swap, config.Load, config.GPU, config.Containers, config.Processes, config.Uptime,
		config.Enabled, config.GlancesURL, config.CPU, config.Memory, config.Network, config.Temp, config.Storage,
		config.Swap, config.Load, config.GPU, config.Containers, config.Processes, config.Uptime,
	)
	return err
}
