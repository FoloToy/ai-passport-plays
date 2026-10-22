package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/FoloToy/ai-passport-plays/internal/firmware"
	"github.com/FoloToy/ai-passport-plays/internal/library"
)

// The community limits firmware downloads per IP. On HTTP 429 every firmware
// worker pauses until the throttle expires; repeated refusals back off further.
var (
	throttleBase = 30 * time.Second
	throttleMax  = 10 * time.Minute
)

// CodeRateLimited is reported when an on-demand download hits the server limit.
const CodeRateLimited = "rate_limited"

type throttle struct {
	mu      sync.Mutex
	until   time.Time
	strikes int
}

func (t *throttle) Until() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.until
}

func (t *throttle) wait(ctx context.Context) error {
	for {
		d := time.Until(t.Until())
		if d <= 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(min(d, time.Second)):
		}
	}
}

func (t *throttle) hit(retryAfter time.Duration) time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.strikes++
	d := throttleBase << min(t.strikes-1, 6)
	d = min(d, throttleMax)
	if retryAfter > d {
		d = retryAfter
	}
	if until := time.Now().Add(d); until.After(t.until) {
		t.until = until
	}
	return t.until
}

func (t *throttle) ok() {
	t.mu.Lock()
	t.strikes = 0
	t.mu.Unlock()
}

type fwQueue struct {
	mu    sync.Mutex
	items []*remotePlay
}

func (q *fwQueue) pop(prefer string) *remotePlay {
	q.mu.Lock()
	defer q.mu.Unlock()
	if prefer != "" {
		for i, p := range q.items {
			if p.Slug == prefer {
				q.items = append(q.items[:i:i], q.items[i+1:]...)
				return p
			}
		}
	}
	if len(q.items) == 0 {
		return nil
	}
	p := q.items[0]
	q.items = q.items[1:]
	return p
}

func (q *fwQueue) pushFront(p *remotePlay) {
	q.mu.Lock()
	q.items = append([]*remotePlay{p}, q.items...)
	q.mu.Unlock()
}

func (m *Manager) runFirmwareQueue(ctx context.Context, lib library.Library, items []*remotePlay, before map[string]*metaFile) {
	q := &fwQueue{items: append([]*remotePlay(nil), items...)}
	prio := make(chan string, 32)
	m.mu.Lock()
	m.priority = prio
	m.mu.Unlock()
	var wg sync.WaitGroup
	for w := 0; w < max(1, m.FirmwareWorkers); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				if m.throttle.wait(ctx) != nil {
					return
				}
				prefer := ""
				select {
				case prefer = <-prio:
				default:
				}
				p := q.pop(prefer)
				if p == nil {
					return
				}
				title := p.Title.Zh
				m.set(func(s *State) { s.Current = append(s.Current, title) })
				err := m.fetchFirmware(ctx, lib, p)
				m.set(func(s *State) { s.Current = removeOne(s.Current, title) })
				if ra, limited := IsRateLimited(err); limited {
					until := m.throttle.hit(ra)
					q.pushFront(p)
					m.set(func(s *State) { s.ThrottledUntil = until })
					continue
				}
				if ctx.Err() != nil {
					return
				}
				m.throttle.ok()
				m.fetchDone(p.Slug, err)
				prev := before[p.Slug]
				m.set(func(s *State) {
					s.ThrottledUntil = time.Time{}
					s.Done++
					s.FirmwareDone++
					switch {
					case err != nil:
						s.Failed++
						if len(s.Failures) < 500 {
							s.Failures = append(s.Failures, Failure{Slug: p.Slug, Title: p.Title, Error: err.Error()})
						}
					case prev == nil || prev.Local.SHA256 == "":
						s.Added++
					default:
						s.Updated++
					}
				})
			}
		}()
	}
	wg.Wait()
}

// fetchFirmware downloads, verifies and records one firmware image.
func (m *Manager) fetchFirmware(ctx context.Context, lib library.Library, p *remotePlay) error {
	fw := p.Firmware
	verify := func(tmp string) error {
		return firmware.Verify(tmp, firmware.Expect{Size: fw.Size, SHA256: fw.SHA256})
	}
	if err := m.download(ctx, lib, fw.URL, lib.FirmwarePath(p.Slug), firmware.MaxSize, verify, false); err != nil {
		return err
	}
	if err := m.updateMeta(lib, p.Slug, nil, func(l *localState) {
		l.SHA256 = strings.ToLower(fw.SHA256)
		l.FirmwareSize = fw.Size
		l.RevisionID = p.RevisionID
	}); err != nil {
		return err
	}
	m.markReady(p.Slug)
	return nil
}

func (m *Manager) fetchDone(slug string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fetch.Slug != slug || !m.fetch.Running {
		return
	}
	m.fetch.Running = false
	if err != nil {
		m.fetch.ErrorCode, m.fetch.Error = CodeOther, err.Error()
	} else {
		m.fetch.Done = true
	}
}

// FetchOne downloads a single play's firmware now, e.g. right before flashing.
// During a sync it moves the play to the front of the firmware queue instead.
func (m *Manager) FetchOne(root, slug string) error {
	lib := library.Library{Root: root}
	if !library.ValidSlug(slug) || root == "" {
		return errors.New("invalid play")
	}
	mf := readMeta(lib, slug)
	if mf == nil {
		return errors.New("play not found")
	}
	var p remotePlay
	if err := json.Unmarshal(mf.Remote, &p); err != nil {
		return err
	}
	p.raw = mf.Remote
	if !hasFirmware(&p) {
		return ErrNoFirmware
	}
	if firmwareCurrent(lib, &p, mf) {
		m.markReady(slug)
		m.mu.Lock()
		m.fetch = FetchState{Slug: slug, Done: true}
		m.mu.Unlock()
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fetch.Running {
		return errors.New(CodeBusy)
	}
	if m.state.Running {
		if m.state.Phase != "firmware" || m.priority == nil {
			return errors.New(CodeBusy)
		}
		select {
		case m.priority <- slug:
		default:
			return errors.New(CodeBusy)
		}
		m.fetch = FetchState{Slug: slug, Running: true}
		return nil
	}
	m.fetch = FetchState{Slug: slug, Running: true}
	go m.fetchAlone(lib, &p)
	return nil
}

func (m *Manager) fetchAlone(lib library.Library, p *remotePlay) {
	if until := m.throttle.Until(); time.Until(until) > 0 {
		m.mu.Lock()
		m.fetch = FetchState{Slug: p.Slug, ErrorCode: CodeRateLimited, Error: "rate limited", ThrottledUntil: until}
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	err := m.fetchFirmware(ctx, lib, p)
	m.mu.Lock()
	defer m.mu.Unlock()
	switch ra, limited := IsRateLimited(err); {
	case limited:
		until := m.throttle.hit(ra)
		m.fetch = FetchState{Slug: p.Slug, ErrorCode: CodeRateLimited, Error: err.Error(), ThrottledUntil: until}
	case err != nil:
		m.fetch = FetchState{Slug: p.Slug, ErrorCode: CodeNetwork, Error: err.Error()}
	default:
		m.throttle.ok()
		m.fetch = FetchState{Slug: p.Slug, Done: true}
	}
}

// ErrNoFirmware marks a play the community lists without a downloadable firmware.
var ErrNoFirmware = errors.New("firmware not available")
