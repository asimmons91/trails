package config

import "embed"

//go:embed application.toml
//go:embed credentials
//go:embed environments
//go:embed importmap.toml
var ConfigFS embed.FS

type Config struct {
	Views    ViewsConfig    `toml:"views"`
	Server   ServerConfig   `toml:"server"`
	Database DatabaseConfig `toml:"database"`
	Mailer   MailerConfig   `toml:"mailer"`
}

type ViewsConfig struct {
	LayoutName string `toml:"layout"`
}

type ServerConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

type DatabaseConfig struct {
	Path   string   `toml:"path"`
	Pragma []string `toml:"pragma"`
}

type MailerConfig struct {
	Delivery string     `toml:"delivery"` // "log" or "smtp"
	From     string     `toml:"from"`
	SMTP     SMTPConfig `toml:"smtp"`
}

type SMTPConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Username string `toml:"username"`
	Password string `toml:"password"`
}
