package cache

import (
	"context"
	"fmt"
	"html/template"
	"time"

	trails "github.com/asimmons91/trails"
)

// FetchFragment renders the named template block via c.RenderBlock(block,
// data), caching the resulting HTML at key for ttl, or returning it
// directly on a hit. The cache key is key alone — a hit is served
// regardless of whether data changed since the entry was written, so key
// must incorporate anything the rendered block depends on (e.g. a
// record's updated_at or id) or a stale fragment can be served.
func FetchFragment(ctx context.Context, c *trails.Context, store Store, block, key string, ttl time.Duration, data any) (template.HTML, error) {
	if raw, ok, err := store.Read(ctx, key); err != nil {
		return "", fmt.Errorf("cache: reading fragment %q: %w", key, err)
	} else if ok {
		return template.HTML(raw), nil
	}

	html, err := c.RenderBlock(block, data)
	if err != nil {
		return "", err
	}

	if err := store.Write(ctx, key, []byte(html), ttl); err != nil {
		return "", fmt.Errorf("cache: writing fragment %q: %w", key, err)
	}

	return html, nil
}
