// Package syncer downloads the community catalog, images and firmware into the library.
//
// A sync runs in two stages so the library becomes usable quickly:
//  1. catalog: list every play, download images, write index.json;
//  2. firmware: download firmware through a queue that pauses together when the
//     server rate-limits (HTTP 429) and accepts priority requests from the UI.
package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FoloToy/ai-passport-desktop/pkg/firmware"
	"github.com/FoloToy/ai-passport-desktop/pkg/library"
	"github.com/FoloToy/ai-passport-desktop/pkg/settings"
)

// Error codes shown to the UI, which translates them.
const (
	CodeNetwork   = "network"
	CodeDiskSpace = "disk_space"
	CodeBusy      = "busy"
	CodeTarget    = "target"
	CodeOther     = "other"
)

type Failure struct {
	Slug  string       `json:"slug"`
	Title library.Text `json:"title"`
	Error string       `json:"error"`
}

type State struct {
	Running   bool   `json:"running"`
	Kind      string `json:"kind"`  // "sync" or "move"
	Phase     string `json:"phase"` // listing, topics, catalog, firmware, indexing, moving, done, failed, cancelled
	ErrorCode string `json:"errorCode,omitempty"`
	Error     string `json:"error,omitempty"`
	// Total and Done count the items of the current phase.
	Total int `json:"total"`
	Done  int `json:"done"`
	// Firmware counters for the whole sync.
	FirmwareTotal  int       `json:"firmwareTotal"`
	FirmwareDone   int       `json:"firmwareDone"`
	Failed         int       `json:"failed"`
	Added          int       `json:"added"`
	Updated        int       `json:"updated"`
	Removed        int       `json:"removed"`
	Failures       []Failure `json:"failures"`
	BytesDone      int64     `json:"bytesDone"`
	BytesTotal     int64     `json:"bytesTotal"`
	Speed          float64   `json:"speed"`
	Current        []string  `json:"current"`
	ThrottledUntil time.Time `json:"throttledUntil"`
	StartedAt      time.Time `json:"startedAt"`
	FinishedAt     time.Time `json:"finishedAt"`
}

type UpdateInfo struct {
	Checking  bool      `json:"checking"`
	Available bool      `json:"available"`
	Changes   int       `json:"changes"`
	CheckedAt time.Time `json:"checkedAt"`
	Error     string    `json:"error,omitempty"`
}

// FetchState describes a single on-demand firmware download started from the UI.
type FetchState struct {
	Slug           string    `json:"slug,omitempty"`
	Running        bool      `json:"running"`
	Done           bool      `json:"done"`
	ErrorCode      string    `json:"errorCode,omitempty"`
	Error          string    `json:"error,omitempty"`
	ThrottledUntil time.Time `json:"throttledUntil"`
}

type metaFile struct {
	Remote json.RawMessage `json:"remote"`
	Local  localState      `json:"local"`
}

type localState struct {
	RevisionID   int               `json:"revisionId"`
	SHA256       string            `json:"sha256"`
	FirmwareSize int64             `json:"firmwareSize"`
	Assets       map[string]string `json:"assets"` // file name in play dir -> remote URL
	Avatar       string            `json:"avatar,omitempty"`
	SyncedAt     time.Time         `json:"syncedAt"`
}

type Manager struct {
	Client *Client
	// CatalogWorkers download images in parallel; FirmwareWorkers share one throttle.
	CatalogWorkers  int
	FirmwareWorkers int
	// OnIndex runs after index.json has been written.
	OnIndex func()

	mu       sync.Mutex
	state    State
	update   UpdateInfo
	fetch    FetchState
	cancel   context.CancelFunc
	bytes    atomic.Int64
	avatars  sync.Map
	metaMu   sync.Mutex
	throttle throttle

	// ready lists slugs whose firmware became available since index.json was last written.
	ready        map[string]bool
	readyVersion int
	priority     chan string
}

func New(client *Client) *Manager {
	return &Manager{Client: client, CatalogWorkers: 8, FirmwareWorkers: 2, state: State{Phase: "idle"}, ready: map[string]bool{}}
}

func (m *Manager) Snapshot() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	s.BytesDone = m.bytes.Load()
	s.Failures = append([]Failure{}, s.Failures...)
	s.Current = append([]string{}, s.Current...)
	return s
}

func (m *Manager) Update() UpdateInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.update
}

func (m *Manager) Fetch() FetchState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fetch
}

func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.Running
}

// Ready returns slugs downloaded since the index was written, with a version that changes on every addition.
func (m *Manager) Ready() ([]string, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.ready))
	for s := range m.ready {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, m.readyVersion
}

// ResetReady forgets the overlay, e.g. after switching library folders.
func (m *Manager) ResetReady() {
	m.mu.Lock()
	m.ready = map[string]bool{}
	m.readyVersion++
	m.mu.Unlock()
}

func (m *Manager) markReady(slug string) {
	m.mu.Lock()
	m.ready[slug] = true
	m.readyVersion++
	m.mu.Unlock()
}

func (m *Manager) set(change func(*State)) {
	m.mu.Lock()
	change(&m.state)
	m.mu.Unlock()
}

func (m *Manager) begin(kind string) (context.Context, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Running || m.fetch.Running {
		return nil, errors.New(CodeBusy)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.bytes.Store(0)
	m.state = State{Running: true, Kind: kind, Phase: "listing", StartedAt: time.Now(), Failures: []Failure{}}
	if kind == "move" {
		m.state.Phase = "moving"
	}
	go m.measureSpeed(ctx)
	return ctx, nil
}

func (m *Manager) finish(phase, code string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Running = false
	m.state.Phase = phase
	m.state.Current = nil
	m.state.Speed = 0
	m.state.ThrottledUntil = time.Time{}
	m.state.FinishedAt = time.Now()
	m.state.BytesDone = m.bytes.Load()
	m.priority = nil
	if err != nil {
		m.state.ErrorCode = code
		m.state.Error = err.Error()
	}
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Manager) measureSpeed(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	last := m.bytes.Load()
	speed := 0.0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := m.bytes.Load()
			speed = speed*0.7 + float64(now-last)*0.3
			last = now
			m.set(func(s *State) { s.Speed = speed })
		}
	}
}

// Cancel stops the current job. Completed plays are kept and skipped next time.
func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil && m.state.Running {
		m.cancel()
	}
}

// Start begins a full or incremental sync into root.
func (m *Manager) Start(root string) error {
	if root == "" {
		return errors.New(CodeTarget)
	}
	ctx, err := m.begin("sync")
	if err != nil {
		return err
	}
	go m.run(ctx, library.Library{Root: root})
	return nil
}

func (m *Manager) run(ctx context.Context, lib library.Library) {
	for _, d := range []string{lib.Root, filepath.Join(lib.Root, "plays"), filepath.Join(lib.Root, "avatars"), lib.TempDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			m.finish("failed", CodeTarget, err)
			return
		}
	}

	// Stage 1: catalog.
	plays, first, err := m.Client.listAll(ctx, url.Values{}, func(done, total int) {
		m.set(func(s *State) { s.Done, s.Total = done, total })
	})
	if err != nil {
		m.stop(ctx, err)
		return
	}
	prev, _ := lib.LoadIndex()
	m.set(func(s *State) { s.Phase = "topics"; s.Done, s.Total = 0, 0 })
	topics := m.fetchTopics(ctx, first, prev)
	recs, err := m.Client.recommendations(ctx)
	if err != nil && prev != nil {
		recs = prev.Recommendations
	}
	if ctx.Err() != nil {
		m.stop(ctx, ctx.Err())
		return
	}

	var valid []*remotePlay
	for i := range plays {
		if library.ValidSlug(plays[i].Slug) {
			valid = append(valid, &plays[i])
		}
	}
	before := map[string]*metaFile{}
	var need int64
	var queue []*remotePlay
	for _, p := range valid {
		mf := readMeta(lib, p.Slug)
		before[p.Slug] = mf
		if hasFirmware(p) && !firmwareCurrent(lib, p, mf) {
			need += p.Firmware.Size
			queue = append(queue, p)
		}
	}
	if free, ok := freeSpace(lib.Root); ok && need > 0 && free < uint64(need)+uint64(len(valid))*400<<10+200<<20 {
		m.finish("failed", CodeDiskSpace, fmt.Errorf("need about %d MB, %d MB free", (need>>20)+200, free>>20))
		return
	}

	m.set(func(s *State) { s.Phase = "catalog"; s.Total = len(valid); s.Done = 0 })
	m.forEach(ctx, valid, m.CatalogWorkers, func(p *remotePlay) {
		title := p.Title.Zh
		m.set(func(s *State) { s.Current = append(s.Current, title) })
		m.syncCatalog(ctx, lib, p)
		m.set(func(s *State) {
			s.Current = removeOne(s.Current, title)
			if ctx.Err() == nil {
				s.Done++
			}
		})
	})
	if ctx.Err() != nil {
		m.writeIndex(lib, valid, first, topics, recs, false)
		m.finish("cancelled", "", nil)
		return
	}
	if err := m.writeIndex(lib, valid, first, topics, recs, len(queue) == 0); err != nil {
		m.finish("failed", CodeTarget, err)
		return
	}

	// Stage 2: firmware. Popular plays first so the most wanted ones are ready soonest.
	sort.SliceStable(queue, func(i, j int) bool { return queue[i].Downloads > queue[j].Downloads })
	m.set(func(s *State) {
		s.Phase = "firmware"
		s.Total, s.Done = len(queue), 0
		s.FirmwareTotal, s.FirmwareDone = len(queue), 0
		s.BytesTotal = need
	})
	m.runFirmwareQueue(ctx, lib, queue, before)

	m.set(func(s *State) { s.Phase = "indexing" })
	complete := ctx.Err() == nil && m.Snapshot().Failed == 0
	if err := m.writeIndex(lib, valid, first, topics, recs, complete); err != nil {
		m.finish("failed", CodeTarget, err)
		return
	}
	_ = os.RemoveAll(lib.TempDir())
	if ctx.Err() != nil {
		m.finish("cancelled", "", nil)
		return
	}
	m.mu.Lock()
	m.update = UpdateInfo{CheckedAt: time.Now()}
	m.mu.Unlock()
	m.finish("done", "", nil)
}

func (m *Manager) writeIndex(lib library.Library, valid []*remotePlay, first *listResponse, topics []library.Topic, recs []int, complete bool) error {
	locals := map[string]localState{}
	for _, p := range valid {
		if mf := readMeta(lib, p.Slug); mf != nil {
			locals[p.Slug] = mf.Local
		}
	}
	idx, removed := buildIndex(lib, valid, locals, first, topics, recs, complete, m.Client.Origin)
	m.set(func(s *State) { s.Removed = removed })
	if err := lib.SaveIndex(idx); err != nil {
		return err
	}
	m.ResetReady()
	if m.OnIndex != nil {
		m.OnIndex()
	}
	return nil
}

// forEach runs fn over items with n workers until done or ctx ends.
func (m *Manager) forEach(ctx context.Context, items []*remotePlay, n int, fn func(*remotePlay)) {
	jobs := make(chan *remotePlay)
	var wg sync.WaitGroup
	for w := 0; w < max(1, n); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				fn(p)
			}
		}()
	}
	for _, p := range items {
		select {
		case <-ctx.Done():
		case jobs <- p:
			continue
		}
		break
	}
	close(jobs)
	wg.Wait()
}

func (m *Manager) stop(ctx context.Context, err error) {
	if ctx.Err() != nil {
		m.finish("cancelled", "", nil)
		return
	}
	m.finish("failed", CodeNetwork, err)
}

func removeOne(list []string, v string) []string {
	for i, x := range list {
		if x == v {
			return append(list[:i:i], list[i+1:]...)
		}
	}
	return list
}

type topicDef struct {
	key, tag string
	name     library.Text
}

func (m *Manager) fetchTopics(ctx context.Context, first *listResponse, prev *library.Index) []library.Topic {
	var defs []topicDef
	for _, t := range first.DiscoveryTags {
		if t.Enabled && t.Key == "must-play" {
			defs = append(defs, topicDef{key: t.Key, tag: t.Key, name: t.Name})
		}
	}
	for _, t := range first.DiscoveryTabs {
		if t.Enabled && t.Visible && strings.HasPrefix(t.Key, "entry:") {
			defs = append(defs, topicDef{key: strings.TrimPrefix(t.Key, "entry:"), tag: t.Key, name: t.Name})
		}
	}
	old := map[string][]int{}
	if prev != nil {
		for _, t := range prev.Topics {
			old[t.Key] = t.IDs
		}
	}
	var out []library.Topic
	for _, d := range defs {
		ids, err := m.Client.listIDs(ctx, url.Values{"tag": {d.tag}})
		if err != nil {
			ids = old[d.key]
		}
		if ids == nil {
			ids = []int{}
		}
		out = append(out, library.Topic{Key: d.key, Tag: d.tag, Name: d.name, IDs: ids})
	}
	return out
}

func readMeta(lib library.Library, slug string) *metaFile {
	b, err := os.ReadFile(lib.MetaPath(slug))
	if err != nil {
		return nil
	}
	var mf metaFile
	if json.Unmarshal(b, &mf) != nil {
		return nil
	}
	return &mf
}

// updateMeta read-modify-writes a play's meta.json under a lock shared by all writers.
func (m *Manager) updateMeta(lib library.Library, slug string, raw json.RawMessage, change func(*localState)) error {
	m.metaMu.Lock()
	defer m.metaMu.Unlock()
	mf := readMeta(lib, slug)
	if mf == nil {
		mf = &metaFile{}
	}
	if raw != nil {
		mf.Remote = raw
	}
	if mf.Local.Assets == nil {
		mf.Local.Assets = map[string]string{}
	}
	change(&mf.Local)
	mf.Local.SyncedAt = time.Now().UTC()
	return settings.WriteJSONAtomic(lib.MetaPath(slug), mf)
}

func fileSize(p string) int64 {
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return -1
	}
	return st.Size()
}

func hasFirmware(p *remotePlay) bool {
	fw := p.Firmware
	return fw != nil && fw.Available && fw.URL != "" && (fw.Format == "" || fw.Format == firmware.Format)
}

func firmwareCurrent(lib library.Library, p *remotePlay, mf *metaFile) bool {
	if p.Firmware == nil || mf == nil {
		return false
	}
	return strings.EqualFold(mf.Local.SHA256, p.Firmware.SHA256) && fileSize(lib.FirmwarePath(p.Slug)) == p.Firmware.Size
}

// LocalFirmware reports the verified firmware recorded for slug, read from disk.
func LocalFirmware(lib library.Library, slug string) (sha string, size int64, ok bool) {
	mf := readMeta(lib, slug)
	if mf == nil || mf.Local.SHA256 == "" || fileSize(lib.FirmwarePath(slug)) != mf.Local.FirmwareSize {
		return "", 0, false
	}
	return mf.Local.SHA256, mf.Local.FirmwareSize, true
}

// CheckUpdates compares the newest page of the community with the local index.
func (m *Manager) CheckUpdates(ctx context.Context, idx *library.Index) {
	m.mu.Lock()
	if m.update.Checking || m.state.Running {
		m.mu.Unlock()
		return
	}
	m.update.Checking = true
	m.mu.Unlock()
	info := UpdateInfo{CheckedAt: time.Now()}
	r, err := m.Client.page(ctx, url.Values{}, 0)
	if err != nil {
		info.Error = err.Error()
	} else if idx != nil {
		known := map[int]int{}
		listed := 0
		for _, p := range idx.Plays {
			if !p.Removed {
				known[p.ID] = p.RevisionID
				listed++
			}
		}
		for _, p := range r.Plays {
			if rev, ok := known[p.ID]; !ok || rev != p.RevisionID {
				info.Changes++
			}
		}
		if diff := r.Pagination.Total - listed; diff > info.Changes {
			info.Changes = diff
		}
		info.Available = info.Changes > 0
	}
	m.mu.Lock()
	m.update = info
	m.mu.Unlock()
}
