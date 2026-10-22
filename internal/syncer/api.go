package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/FoloToy/ai-passport-plays/internal/library"
)

// DefaultOrigin is the AI Passport community site.
const DefaultOrigin = "https://ai-passport.folotoy.cn"

const pageSize = 50 // server maximum

type remoteFirmware struct {
	Available bool   `json:"available"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	URL       string `json:"url"`
	Format    string `json:"format"`
}

type remoteAuthor struct {
	Name       string `json:"name"`
	IsOfficial bool   `json:"isOfficial"`
	AvatarURL  string `json:"avatarUrl"`
}

type remotePlay struct {
	ID            int             `json:"id"`
	Slug          string          `json:"slug"`
	Source        string          `json:"source"`
	RevisionID    int             `json:"revisionId"`
	Title         library.Text    `json:"title"`
	Description   library.Text    `json:"description"`
	Instructions  library.Text    `json:"instructions"`
	Changelog     library.Text    `json:"changelog"`
	CategoryKey   string          `json:"categoryKey"`
	Category      string          `json:"category"`
	Tags          []string        `json:"tags"`
	MustPlay      bool            `json:"mustPlay"`
	MultiDevice   bool            `json:"multiDevice"`
	ChildFriendly bool            `json:"childFriendly"`
	IsOfficial    bool            `json:"isOfficial"`
	Author        string          `json:"author"`
	AuthorProfile *remoteAuthor   `json:"authorProfile"`
	Image         string          `json:"image"`
	Thumbnail     string          `json:"thumbnail"`
	Images        []string        `json:"images"`
	PublishedAt   string          `json:"publishedAt"`
	UpdatedAt     *string         `json:"updatedAt"`
	Downloads     int             `json:"downloads"`
	Hearts        int             `json:"hearts"`
	Favorites     int             `json:"favorites"`
	CommentCount  int             `json:"commentCount"`
	Views         int             `json:"views"`
	DownloadURL   string          `json:"downloadUrl"`
	Firmware      *remoteFirmware `json:"firmware"`

	raw json.RawMessage
}

func (p *remotePlay) UnmarshalJSON(b []byte) error {
	type plain remotePlay
	var v plain
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*p = remotePlay(v)
	p.raw = append(json.RawMessage(nil), b...)
	return nil
}

func (p *remotePlay) category() string {
	if p.CategoryKey != "" {
		return p.CategoryKey
	}
	return p.Category
}

type discoveryTab struct {
	Key     string       `json:"key"`
	Name    library.Text `json:"name"`
	Enabled bool         `json:"enabled"`
	Visible bool         `json:"visible"`
}

type discoveryTag struct {
	Key     string       `json:"key"`
	Name    library.Text `json:"name"`
	Enabled bool         `json:"enabled"`
}

type listResponse struct {
	OK         bool         `json:"ok"`
	Plays      []remotePlay `json:"plays"`
	Pagination struct {
		Total   int  `json:"total"`
		HasMore bool `json:"hasMore"`
	} `json:"pagination"`
	CategoryCounts map[string]int `json:"categoryCounts"`
	DiscoveryTags  []discoveryTag `json:"discoveryTags"`
	DiscoveryTabs  []discoveryTab `json:"discoveryTabs"`
	Taxonomy       string         `json:"taxonomyVersion"`
	Bootstrap      *struct {
		Recommendations []struct {
			ID int `json:"id"`
		} `json:"recommendations"`
	} `json:"bootstrap"`
}

// HTTPError carries the status so callers can decide whether to retry.
type HTTPError struct {
	Status     int
	URL        string
	RetryAfter time.Duration
}

// IsRateLimited reports whether err is an HTTP 429 from the server.
func IsRateLimited(err error) (time.Duration, bool) {
	var he *HTTPError
	if errors.As(err, &he) && he.Status == http.StatusTooManyRequests {
		return he.RetryAfter, true
	}
	return 0, false
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.URL) }

type Client struct {
	Origin    string
	HTTP      *http.Client
	UserAgent string
}

func NewClient(origin, version string) *Client {
	if origin == "" {
		origin = DefaultOrigin
	}
	return &Client{
		Origin:    origin,
		HTTP:      &http.Client{Timeout: 3 * time.Minute},
		UserAgent: "AIPassportPlays/" + version + " (offline desktop)",
	}
}

// absolute resolves a server-relative path against the origin; absolute URLs on other hosts are refused.
func (c *Client) absolute(ref string) (string, error) {
	base, err := url.Parse(c.Origin)
	if err != nil {
		return "", err
	}
	u, err := base.Parse(ref)
	if err != nil {
		return "", err
	}
	if u.Host != base.Host || u.Scheme != base.Scheme {
		return "", fmt.Errorf("refusing off-site URL %q", ref)
	}
	return u.String(), nil
}

func retryable(err error, retry429 bool) bool {
	var pe *permanent
	if errors.As(err, &pe) {
		return false
	}
	if _, limited := IsRateLimited(err); limited {
		return retry429
	}
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Status == 429 || he.Status >= 500
	}
	return true
}

// withRetry runs fn up to three times with backoff unless ctx ends or the error is permanent.
func withRetry(ctx context.Context, fn func() error) error { return retryLoop(ctx, true, fn) }

// retryLoop is withRetry; with retry429 false a rate-limit error is returned at once
// so the firmware queue can pause every worker together.
func retryLoop(ctx context.Context, retry429 bool, fn func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt*attempt) * 1500 * time.Millisecond
			if ra, limited := IsRateLimited(err); limited {
				wait = max(ra, time.Duration(attempt)*10*time.Second)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		if err = fn(); err == nil || ctx.Err() != nil || !retryable(err, retry429) {
			break
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// open issues a GET and returns the body for a 200 response.
func (c *Client) open(ctx context.Context, ref string) (*http.Response, error) {
	u, err := c.absolute(ref)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &HTTPError{Status: resp.StatusCode, URL: u, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	return resp, nil
}

func (c *Client) getJSON(ctx context.Context, ref string, v any) error {
	return withRetry(ctx, func() error {
		resp, err := c.open(ctx, ref)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(v)
	})
}

func (c *Client) page(ctx context.Context, q url.Values, offset int) (*listResponse, error) {
	q = cloneValues(q)
	q.Set("limit", strconv.Itoa(pageSize))
	q.Set("offset", strconv.Itoa(offset))
	var r listResponse
	if err := c.getJSON(ctx, "/api/plays?"+q.Encode(), &r); err != nil {
		return nil, err
	}
	if !r.OK {
		return nil, errors.New("community API returned an error")
	}
	return &r, nil
}

func cloneValues(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// listAll pages through a list query. The first page's response is returned for its metadata.
func (c *Client) listAll(ctx context.Context, q url.Values, onPage func(done, total int)) ([]remotePlay, *listResponse, error) {
	seen := map[int]bool{}
	var out []remotePlay
	var first *listResponse
	for pass := 0; pass < 2; pass++ {
		for offset := 0; ; offset += pageSize {
			r, err := c.page(ctx, q, offset)
			if err != nil {
				return nil, nil, err
			}
			if first == nil {
				first = r
			}
			for _, p := range r.Plays {
				if !seen[p.ID] {
					seen[p.ID] = true
					out = append(out, p)
				}
			}
			if onPage != nil {
				onPage(len(out), r.Pagination.Total)
			}
			if !r.Pagination.HasMore || len(r.Plays) == 0 {
				break
			}
		}
		// Items published or removed while paging can shift offsets; one extra pass fills gaps.
		if len(out) >= first.Pagination.Total {
			break
		}
	}
	return out, first, nil
}

// listIDs returns only the ids for a filtered list (used for topics).
func (c *Client) listIDs(ctx context.Context, q url.Values) ([]int, error) {
	plays, _, err := c.listAll(ctx, q, nil)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(plays))
	for _, p := range plays {
		ids = append(ids, p.ID)
	}
	return ids, nil
}

func (c *Client) recommendations(ctx context.Context) ([]int, error) {
	var r listResponse
	if err := c.getJSON(ctx, "/api/plays?bootstrap=true&limit=1", &r); err != nil {
		return nil, err
	}
	var ids []int
	if r.Bootstrap != nil {
		for _, p := range r.Bootstrap.Recommendations {
			ids = append(ids, p.ID)
		}
	}
	return ids, nil
}
