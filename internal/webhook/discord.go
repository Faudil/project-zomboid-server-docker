package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/faudil/project-zomboid-server-docker/internal/config"
)

type DiscordWebhook struct {
	cfg    *config.ServerConfig
	client *http.Client
}

type discordEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       int    `json:"color"`
	Timestamp   string `json:"timestamp"`
	Footer      struct {
		Text string `json:"text"`
	} `json:"footer"`
}

type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

func NewDiscord(cfg *config.ServerConfig) *DiscordWebhook {
	if cfg.DiscordURL == "" {
		return nil
	}
	return &DiscordWebhook{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *DiscordWebhook) Send(title, description string, color int) error {
	if d == nil || d.cfg.DiscordURL == "" {
		return nil
	}

	payload := discordPayload{
		Embeds: []discordEmbed{
			{
				Title:       title,
				Description: description,
				Color:       color,
				Timestamp:   time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	payload.Embeds[0].Footer.Text = fmt.Sprintf("Server: %s", d.cfg.ServerName)

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := d.client.Post(d.cfg.DiscordURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *DiscordWebhook) NotifyStart() {
	if !d.cfg.DiscordStart {
		return
	}
	_ = d.Send("\U0001f7e2 Server Started", fmt.Sprintf("**%s** is now online", d.cfg.PublicName), 0x57F287)
}

func (d *DiscordWebhook) NotifyStop() {
	if !d.cfg.DiscordStop {
		return
	}
	_ = d.Send("\U0001f534 Server Stopped", fmt.Sprintf("**%s** has shut down", d.cfg.PublicName), 0xED4245)
}

func (d *DiscordWebhook) NotifyCrash(err error) {
	if !d.cfg.DiscordCrash {
		return
	}
	_ = d.Send("\U0001f4a5 Server Crashed", fmt.Sprintf("**%s** exited unexpectedly: %v", d.cfg.PublicName, err), 0xFEE75C)
}
