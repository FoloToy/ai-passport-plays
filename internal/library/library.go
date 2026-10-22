// Package library owns the on-disk layout of the play library chosen by the user:
//
//	index.json                 catalog used by the UI
//	plays/<slug>/meta.json     raw community record for this play
//	plays/<slug>/cover.webp    thumb.webp  gallery/N.webp
//	plays/<slug>/firmware.bin  verified firmware image
//	avatars/<name>.webp        author avatars
//	.tmp/                      in-progress downloads
package library

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/FoloToy/ai-passport-plays/internal/settings"
)

const IndexVersion = 1

type Text struct {
	Zh string `json:"zh"`
	En string `json:"en"`
}

type Author struct {
	Name     string `json:"name"`
	Avatar   string `json:"avatar,omitempty"` // library-relative path
	Official bool   `json:"official,omitempty"`
}

type Firmware struct {
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Format string `json:"format"`
}

// Play is the normalized record the UI reads. Paths are relative to the library root.
type Play struct {
	ID            int      `json:"id"`
	Slug          string   `json:"slug"`
	Source        string   `json:"source"`
	RevisionID    int      `json:"revisionId"`
	Title         Text     `json:"title"`
	Description   Text     `json:"description"`
	Instructions  Text     `json:"instructions"`
	Changelog     Text     `json:"changelog"`
	Category      string   `json:"category"`
	Tags          []string `json:"tags"`
	MustPlay      bool     `json:"mustPlay"`
	MultiDevice   bool     `json:"multiDevice"`
	ChildFriendly bool     `json:"childFriendly"`
	Official      bool     `json:"official"`
	Author        Author   `json:"author"`
	PublishedAt   string   `json:"publishedAt"`
	UpdatedAt     string   `json:"updatedAt,omitempty"`
	Downloads     int      `json:"downloads"`
	Hearts        int      `json:"hearts"`
	Favorites     int      `json:"favorites"`
	Comments      int      `json:"comments"`
	Views         int      `json:"views"`
	Cover         string   `json:"cover,omitempty"`
	Thumb         string   `json:"thumb,omitempty"`
	Gallery       []string `json:"gallery"`
	Firmware      Firmware `json:"firmware"`
	// Ready is true when firmware.bin is present and verified.
	Ready bool `json:"ready"`
	// Downloadable is true when the community publishes a firmware for this play.
	Downloadable bool `json:"downloadable"`
	// Removed is true when the play is no longer listed on the community site.
	Removed bool `json:"removed,omitempty"`
}

type Category struct {
	Key   string `json:"key"`
	Icon  string `json:"icon"`
	Name  Text   `json:"name"`
	Count int    `json:"count"`
}

type Topic struct {
	Key  string `json:"key"`
	Tag  string `json:"tag"`
	Name Text   `json:"name"`
	IDs  []int  `json:"ids"`
}

type Index struct {
	Version         int        `json:"version"`
	Origin          string     `json:"origin"`
	SyncedAt        time.Time  `json:"syncedAt"`
	Complete        bool       `json:"complete"`
	Taxonomy        string     `json:"taxonomy,omitempty"`
	Categories      []Category `json:"categories"`
	Topics          []Topic    `json:"topics"`
	Recommendations []int      `json:"recommendations"`
	Plays           []Play     `json:"plays"`
}

// Categories mirrors the sidebar of the community site. The API returns keys and counts only.
var Categories = []Category{
	{Key: "games", Icon: "✦", Name: Text{"游戏与互动", "Games & Interaction"}},
	{Key: "productivity", Icon: "ϟ", Name: Text{"效率与工具", "Productivity & Tools"}},
	{Key: "information", Icon: "▤", Name: Text{"信息与展示", "Information & Display"}},
	{Key: "learning", Icon: "◇", Name: Text{"学习与创作", "Learning & Creation"}},
	{Key: "media", Icon: "♪", Name: Text{"音乐与影像", "Music & Visuals"}},
	{Key: "social", Icon: "◉", Name: Text{"活动与社交", "Events & Social"}},
	{Key: "developer", Icon: "⌘", Name: Text{"开发者实验", "Developer Experiments"}},
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

// ValidSlug rejects anything that could escape the plays directory.
func ValidSlug(s string) bool { return slugPattern.MatchString(s) && !strings.Contains(s, "..") }

type Library struct{ Root string }

func (l Library) IndexPath() string           { return filepath.Join(l.Root, "index.json") }
func (l Library) PlayDir(slug string) string  { return filepath.Join(l.Root, "plays", slug) }
func (l Library) MetaPath(slug string) string { return filepath.Join(l.PlayDir(slug), "meta.json") }
func (l Library) FirmwarePath(slug string) string {
	return filepath.Join(l.PlayDir(slug), "firmware.bin")
}
func (l Library) TempDir() string { return filepath.Join(l.Root, ".tmp") }

func (l Library) HasIndex() bool {
	st, err := os.Stat(l.IndexPath())
	return err == nil && st.Mode().IsRegular()
}

func (l Library) LoadIndex() (*Index, error) {
	b, err := os.ReadFile(l.IndexPath())
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(b, &idx); err != nil {
		return nil, err
	}
	if idx.Version != IndexVersion {
		return nil, errors.New("unsupported index version")
	}
	return &idx, nil
}

func (l Library) SaveIndex(idx *Index) error {
	idx.Version = IndexVersion
	return settings.WriteJSONAtomic(l.IndexPath(), idx)
}

// Resolve maps a library-relative media path to an absolute one,
// allowing only image files inside plays/ or avatars/.
func (l Library) Resolve(rel string) (string, bool) {
	rel = filepath.ToSlash(filepath.Clean("/" + rel))[1:]
	if rel == "" || strings.Contains(rel, "..") {
		return "", false
	}
	if !strings.HasPrefix(rel, "plays/") && !strings.HasPrefix(rel, "avatars/") {
		return "", false
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".webp", ".png", ".jpg", ".jpeg", ".gif":
	default:
		return "", false
	}
	return filepath.Join(l.Root, filepath.FromSlash(rel)), true
}

// Usage returns the total bytes and file count under the library root.
func (l Library) Usage() (int64, int) {
	var total int64
	var files int
	_ = filepath.WalkDir(l.Root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			if info, e := d.Info(); e == nil {
				total += info.Size()
				files++
			}
		}
		return nil
	})
	return total, files
}
