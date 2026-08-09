package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const FileName = "libshelf.json"

type Config struct {
	Addr        string   `json:"addr"`
	LibraryDir  string   `json:"library_dir"`
	LibraryDirs []string `json:"library_dirs,omitempty"`
	DataDir     string   `json:"data_dir"`
	INPX        string   `json:"inpx"`
	Auth        string   `json:"auth"`
	// Languages filters the catalog. Default ["ru"]. Use ["*"] for all languages.
	Languages  []string `json:"languages"`
	OpenBrowser bool    `json:"open_browser"`
}

// AllLibraryDirs returns unique library roots (library_dirs + library_dir).
func (c Config) AllLibraryDirs() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(d string) {
		d = filepath.Clean(d)
		if d == "" || d == "." {
			return
		}
		if _, ok := seen[d]; ok {
			return
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	for _, d := range c.LibraryDirs {
		add(d)
	}
	add(c.LibraryDir)
	return out
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
		Languages:   []string{"ru"},
		OpenBrowser: true,
	}
}

// NormalizeLanguages returns the language filter for the store.
// ["*"] / ["all"] / empty explicit list → nil (all languages).
func NormalizeLanguages(langs []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, raw := range langs {
		l := strings.ToLower(strings.TrimSpace(raw))
		if l == "" {
			continue
		}
		if l == "*" || l == "all" {
			return nil
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out
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
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	cfg.applyDefaults()
	if _, ok := raw["languages"]; !ok {
		cfg.Languages = []string{"ru"}
	} else {
		// Preserve explicit ["*"] or [] as all-languages marker for Save/UI.
		n := NormalizeLanguages(cfg.Languages)
		if n == nil {
			cfg.Languages = []string{"*"}
		} else {
			cfg.Languages = n
		}
	}
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

// StoreLanguages returns languages for store.SetLanguages (nil = all).
func (c Config) StoreLanguages() []string {
	return NormalizeLanguages(c.Languages)
}

func (c Config) Save(path string) error {
	c.applyDefaults()
	if len(c.Languages) == 0 {
		c.Languages = []string{"ru"}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
