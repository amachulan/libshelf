package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const FileName = "libshelf.json"

type Config struct {
	Addr        string `json:"addr"`
	LibraryDir  string `json:"library_dir"`
	DataDir     string `json:"data_dir"`
	INPX        string `json:"inpx"`
	Auth        string `json:"auth"`
	OpenBrowser bool   `json:"open_browser"`
}

func ExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

func DefaultPath() string {
	return filepath.Join(ExeDir(), FileName)
}

func Defaults() Config {
	base := ExeDir()
	return Config{
		Addr:        "127.0.0.1:12380",
		LibraryDir:  filepath.Join(base, "library"),
		DataDir:     filepath.Join(base, "data"),
		INPX:        "",
		Auth:        "users",
		OpenBrowser: true,
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) applyDefaults() {
	d := Defaults()
	if c.Addr == "" {
		c.Addr = d.Addr
	}
	if c.DataDir == "" {
		c.DataDir = d.DataDir
	}
	if c.LibraryDir == "" {
		c.LibraryDir = d.LibraryDir
	}
	if c.Auth == "" {
		c.Auth = d.Auth
	}
}

func (c Config) Save(path string) error {
	c.applyDefaults()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
