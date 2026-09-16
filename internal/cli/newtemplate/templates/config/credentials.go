package config

// Credentials holds secrets that aren't part of Config, decoded via
// trails.LoadCredentials[config.Credentials](env, config.ConfigFS).
// Config's own fields (e.g. DatabaseConfig.Password) are layered from the
// same encrypted file automatically by trails.LoadConfig — see
// `trails credentials edit`.
type Credentials struct {
	SecretKeyBase string `toml:"secret_key_base"`
}
