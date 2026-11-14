package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/FoloToy/ai-passport-desktop/pkg/firmware"
	"github.com/FoloToy/ai-passport-desktop/pkg/library"
)

func extOf(ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ".webp"
	}
	switch e := strings.ToLower(path.Ext(u.Path)); e {
	case ".webp", ".png", ".jpg", ".jpeg", ".gif":
		return e
	}
	return ".webp"
}

func wantedAssets(p *remotePlay) map[string]string {
	want := map[string]string{}
	if p.Image != "" {
		want["cover"+extOf(p.Image)] = p.Image
	}
	if p.Thumbnail != "" {
		want["thumb"+extOf(p.Thumbnail)] = p.Thumbnail
	}
	for i, g := range p.Images {
		if g != "" {
			want["gallery/"+strconv.Itoa(i)+extOf(g)] = g
		}
	}
	return want
}

// syncCatalog stores a play's record and images. Images are best effort.
func (m *Manager) syncCatalog(ctx context.Context, lib library.Library, p *remotePlay) {
	dir := lib.PlayDir(p.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	prev := readMeta(lib, p.Slug)
	have := map[string]string{}
	if prev != nil && prev.Local.Assets != nil {
		have = prev.Local.Assets
	}
	want := wantedAssets(p)
	got := map[string]string{}
	for name, ref := range want {
		if ctx.Err() != nil {
			return
		}
		dest := filepath.Join(dir, filepath.FromSlash(name))
		if have[name] == ref && fileSize(dest) > 0 {
			got[name] = ref
			continue
		}
		if err := m.download(ctx, lib, ref, dest, 20<<20, nil, true); err == nil {
			got[name] = ref
		}
	}
	for name := range have {
		if _, ok := want[name]; !ok {
			_ = os.Remove(filepath.Join(dir, filepath.FromSlash(name)))
		}
	}
	avatar := ""
	if a := p.AuthorProfile; a != nil && a.AvatarURL != "" {
		avatar = m.ensureAvatar(ctx, lib, a.AvatarURL)
	}
	_ = m.updateMeta(lib, p.Slug, p.raw, func(l *localState) {
		l.Assets = got
		l.Avatar = avatar
		if p.Firmware != nil && l.SHA256 != "" && strings.EqualFold(l.SHA256, p.Firmware.SHA256) {
			l.RevisionID = p.RevisionID
		}
	})
}

func (m *Manager) ensureAvatar(ctx context.Context, lib library.Library, ref string) string {
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	name := path.Base(u.Path)
	if !library.ValidSlug(strings.ToLower(name)) {
		return ""
	}
	rel := "avatars/" + name
	dest := filepath.Join(lib.Root, "avatars", name)
	done := make(chan struct{})
	if v, loaded := m.avatars.LoadOrStore(dest, done); loaded {
		<-v.(chan struct{})
	} else {
		if fileSize(dest) <= 0 {
			_ = m.download(ctx, lib, ref, dest, 5<<20, nil, true)
		}
		close(done)
	}
	if fileSize(dest) > 0 {
		return rel
	}
	m.avatars.Delete(dest)
	return ""
}

type countingReader struct {
	r io.Reader
	n *atomic.Int64
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

type permanent struct{ error }

// download fetches ref into a temp file, runs check, then atomically moves it to dest.
// With retry429 false a rate-limit response is returned to the caller immediately.
func (m *Manager) download(ctx context.Context, lib library.Library, ref, dest string, limit int64, check func(string) error, retry429 bool) error {
	return retryLoop(ctx, retry429, func() error {
		resp, err := m.Client.open(ctx, ref)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if err := os.MkdirAll(lib.TempDir(), 0o755); err != nil {
			return err
		}
		f, err := os.CreateTemp(lib.TempDir(), "dl-*")
		if err != nil {
			return err
		}
		tmp := f.Name()
		defer os.Remove(tmp)
		n, err := io.Copy(f, countingReader{io.LimitReader(resp.Body, limit+1), &m.bytes})
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		if n > limit {
			return &permanent{errors.New("file too large")}
		}
		if check != nil {
			if err := check(tmp); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.Rename(tmp, dest)
	})
}

func normalize(p *remotePlay, local localState, slugDir string) library.Play {
	out := library.Play{
		ID: p.ID, Slug: p.Slug, Source: p.Source, RevisionID: p.RevisionID,
		Title: p.Title, Description: p.Description, Instructions: p.Instructions, Changelog: p.Changelog,
		Category: p.category(), Tags: p.Tags, MustPlay: p.MustPlay, MultiDevice: p.MultiDevice,
		ChildFriendly: p.ChildFriendly, Official: p.IsOfficial,
		Author:      library.Author{Name: p.Author, Avatar: local.Avatar},
		PublishedAt: p.PublishedAt, Downloads: p.Downloads, Hearts: p.Hearts, Favorites: p.Favorites,
		Comments: p.CommentCount, Views: p.Views, Gallery: []string{},
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	if p.UpdatedAt != nil {
		out.UpdatedAt = *p.UpdatedAt
	}
	if a := p.AuthorProfile; a != nil {
		out.Author.Official = a.IsOfficial
		if out.Author.Name == "" {
			out.Author.Name = a.Name
		}
	}
	base := "plays/" + p.Slug + "/"
	var gallery []string
	for name := range local.Assets {
		switch {
		case strings.HasPrefix(name, "cover."):
			out.Cover = base + name
		case strings.HasPrefix(name, "thumb."):
			out.Thumb = base + name
		case strings.HasPrefix(name, "gallery/"):
			gallery = append(gallery, name)
		}
	}
	sort.Slice(gallery, func(i, j int) bool { return galleryIndex(gallery[i]) < galleryIndex(gallery[j]) })
	for _, g := range gallery {
		out.Gallery = append(out.Gallery, base+g)
	}
	if local.SHA256 != "" && fileSize(filepath.Join(slugDir, "firmware.bin")) == local.FirmwareSize {
		out.Ready = true
		out.Firmware = library.Firmware{Size: local.FirmwareSize, SHA256: local.SHA256, Format: firmware.Format}
	} else if p.Firmware != nil {
		out.Firmware = library.Firmware{Size: p.Firmware.Size, SHA256: p.Firmware.SHA256, Format: p.Firmware.Format}
	}
	out.Downloadable = hasFirmware(p)
	return out
}

func galleryIndex(name string) int {
	base := strings.TrimPrefix(name, "gallery/")
	n, _ := strconv.Atoi(strings.TrimSuffix(base, path.Ext(base)))
	return n
}

func buildIndex(lib library.Library, plays []*remotePlay, locals map[string]localState, first *listResponse, topics []library.Topic, recs []int, complete bool, origin string) (*library.Index, int) {
	idx := &library.Index{Origin: origin, SyncedAt: time.Now().UTC(), Complete: complete, Taxonomy: first.Taxonomy, Topics: topics, Recommendations: recs}
	if idx.Recommendations == nil {
		idx.Recommendations = []int{}
	}
	if idx.Topics == nil {
		idx.Topics = []library.Topic{}
	}
	listed := map[string]bool{}
	counts := map[string]int{}
	for _, p := range plays {
		listed[p.Slug] = true
		play := normalize(p, locals[p.Slug], lib.PlayDir(p.Slug))
		counts[play.Category]++
		idx.Plays = append(idx.Plays, play)
	}
	removed := 0
	entries, _ := os.ReadDir(filepath.Join(lib.Root, "plays"))
	for _, e := range entries {
		if !e.IsDir() || listed[e.Name()] || !library.ValidSlug(e.Name()) {
			continue
		}
		mf := readMeta(lib, e.Name())
		if mf == nil {
			continue
		}
		var p remotePlay
		if json.Unmarshal(mf.Remote, &p) != nil || p.Slug != e.Name() {
			continue
		}
		play := normalize(&p, mf.Local, lib.PlayDir(p.Slug))
		play.Removed = true
		play.Downloadable = false
		idx.Plays = append(idx.Plays, play)
		removed++
	}
	for _, c := range library.Categories {
		c.Count = counts[c.Key]
		idx.Categories = append(idx.Categories, c)
	}
	if idx.Plays == nil {
		idx.Plays = []library.Play{}
	}
	return idx, removed
}
