package fantlab

import (
	"context"
	"fmt"
	"log"
	"time"

	"libshelf/internal/store"
)

type Options struct {
	Delay       time.Duration
	Limit       int
	Genre       string
	RetryFailed bool
	UserAgent   string
	BaseURL     string
	Searcher    Searcher
}

type Searcher interface {
	SearchWorks(ctx context.Context, query string) ([]Hit, error)
}

type Stats struct {
	Looked    int
	Matched   int
	None      int
	Ambiguous int
	Errors    int
}

func Fetch(ctx context.Context, st *store.Store, opts Options) (Stats, error) {
	if opts.RetryFailed {
		n, err := st.ClearFailedFantLab()
		if err != nil {
			return Stats{}, err
		}
		if n > 0 {
			log.Printf("fantlab: cleared %d failed rows for retry", n)
		}
	}
	works, err := st.PendingFantLabWorks(opts.Genre, opts.Limit)
	if err != nil {
		return Stats{}, err
	}
	searcher := opts.Searcher
	if searcher == nil {
		searcher = &Client{BaseURL: opts.BaseURL, UserAgent: opts.UserAgent}
	}
	delay := opts.Delay
	if delay <= 0 {
		delay = time.Second
	}
	defer st.InvalidateCatalogCache()
	var stats Stats
	for i, w := range works {
		if err := ctx.Err(); err != nil {
			log.Printf("fantlab: stopped (%v) after %d works", err, stats.Looked)
			return stats, err
		}
		q := SearchQuery(w.Title, w.AuthorLasts)
		hits, err := searchWithRetry(ctx, searcher, q)
		if err != nil {
			stats.Errors++
			log.Printf("fantlab: search %q: %v", q, err)
			if i+1 < len(works) {
				if err := sleep(ctx, delay); err != nil {
					return stats, err
				}
			}
			continue
		}
		m := PickMatch(w.Title, w.AuthorLasts, hits)
		row := store.FantLabRating{WorkKey: w.Key, Status: m.Status}
		if m.Status == statusOK {
			row.FantLabID = m.Hit.WorkID
			row.Rating = m.Hit.Midmark
			row.Voters = m.Hit.MarkCount
			row.MatchedTitle = m.Hit.RusName
			if row.MatchedTitle == "" {
				row.MatchedTitle = m.Hit.Name
			}
			stats.Matched++
		} else if m.Status == statusAmbiguous {
			stats.Ambiguous++
		} else {
			stats.None++
		}
		if err := st.UpsertFantLab(row); err != nil {
			return stats, err
		}
		stats.Looked++
		if stats.Looked%50 == 0 || stats.Looked == len(works) {
			log.Printf("fantlab: %d/%d matched=%d none=%d ambiguous=%d errors=%d",
				stats.Looked, len(works), stats.Matched, stats.None, stats.Ambiguous, stats.Errors)
		}
		if i+1 < len(works) {
			if err := sleep(ctx, delay); err != nil {
				return stats, err
			}
		}
	}
	return stats, nil
}

func searchWithRetry(ctx context.Context, searcher Searcher, q string) ([]Hit, error) {
	hits, err := searcher.SearchWorks(ctx, q)
	if err == nil {
		return hits, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	msg := err.Error()
	wait := 3 * time.Second
	if contains429(msg) {
		wait = 15 * time.Second
	}
	if err := sleep(ctx, wait); err != nil {
		return nil, err
	}
	return searcher.SearchWorks(ctx, q)
}

func contains429(s string) bool {
	return len(s) >= 3 && (s == "fantlab 429" || (len(s) >= 11 && s[8:11] == "429"))
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func FormatStats(s Stats) string {
	return fmt.Sprintf("looked=%d matched=%d none=%d ambiguous=%d errors=%d",
		s.Looked, s.Matched, s.None, s.Ambiguous, s.Errors)
}
