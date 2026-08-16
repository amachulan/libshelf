package fantlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBase = "https://api.fantlab.ru"

type Client struct {
	HTTP      *http.Client
	BaseURL   string
	UserAgent string
}

type Hit struct {
	WorkID    int64
	RusName   string
	Name      string
	AltName   string
	Authors   []string
	Midmark   float64
	MarkCount int
}

type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	b = bytesTrim(b)
	if len(b) == 0 || string(b) == "null" {
		*f = 0
		return nil
	}
	if b[0] == '[' {
		var arr []flexFloat
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		if len(arr) > 0 {
			*f = arr[0]
		}
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			*f = 0
			return nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		*f = flexFloat(v)
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*f = flexFloat(v)
	return nil
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

type searchHit struct {
	WorkID        int64     `json:"work_id"`
	RusName       string    `json:"rusname"`
	Name          string    `json:"name"`
	AltName       string    `json:"altname"`
	Autor1Rusname string    `json:"autor1_rusname"`
	Autor2Rusname string    `json:"autor2_rusname"`
	Autor3Rusname string    `json:"autor3_rusname"`
	AllAutorRus   string    `json:"all_autor_rusname"`
	Midmark       flexFloat `json:"midmark"`
	MarkCount     int       `json:"markcount"`
}

func (c *Client) SearchWorks(ctx context.Context, query string) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	base := c.BaseURL
	if base == "" {
		base = defaultBase
	}
	u, err := url.Parse(strings.TrimRight(base, "/") + "/search-works")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("page", "1")
	q.Set("onlymatches", "1")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	ua := c.UserAgent
	if ua == "" {
		ua = "libshelf/fantlab-fetch (https://github.com/amachulan/libshelf)"
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/json")

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("fantlab 429")
	}
	if res.StatusCode >= 500 {
		return nil, fmt.Errorf("fantlab %s", res.Status)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fantlab %s", res.Status)
	}
	var raw []searchHit
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("fantlab decode: %w", err)
	}
	out := make([]Hit, 0, len(raw))
	for _, h := range raw {
		authors := make([]string, 0, 4)
		for _, n := range []string{h.Autor1Rusname, h.Autor2Rusname, h.Autor3Rusname} {
			if strings.TrimSpace(n) != "" {
				authors = append(authors, n)
			}
		}
		if h.AllAutorRus != "" {
			authors = append(authors, h.AllAutorRus)
		}
		out = append(out, Hit{
			WorkID:    h.WorkID,
			RusName:   h.RusName,
			Name:      h.Name,
			AltName:   h.AltName,
			Authors:   authors,
			Midmark:   float64(h.Midmark),
			MarkCount: h.MarkCount,
		})
	}
	return out, nil
}
