// Package config reads gh-dash's config file. The schema and defaults mirror
// dlvhdr/gh-dash (MIT) so an existing config.yml works unchanged.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ViewType string

const (
	NotificationsView ViewType = "notifications"
	PRsView           ViewType = "prs"
	IssuesView        ViewType = "issues"
	RepoView          ViewType = "repo"
)

type ColumnConfig struct {
	Width  *int  `yaml:"width,omitempty"`
	Hidden *bool `yaml:"hidden,omitempty"`
}

type PrsLayoutConfig struct {
	UpdatedAt    ColumnConfig `yaml:"updatedAt,omitempty"`
	CreatedAt    ColumnConfig `yaml:"createdAt,omitempty"`
	Repo         ColumnConfig `yaml:"repo,omitempty"`
	Author       ColumnConfig `yaml:"author,omitempty"`
	AuthorIcon   ColumnConfig `yaml:"authorIcon,omitempty"`
	Labels       ColumnConfig `yaml:"labels,omitempty"`
	Assignees    ColumnConfig `yaml:"assignees,omitempty"`
	Title        ColumnConfig `yaml:"title,omitempty"`
	Base         ColumnConfig `yaml:"base,omitempty"`
	ReviewStatus ColumnConfig `yaml:"reviewStatus,omitempty"`
	State        ColumnConfig `yaml:"state,omitempty"`
	Ci           ColumnConfig `yaml:"ci,omitempty"`
	Lines        ColumnConfig `yaml:"lines,omitempty"`
	NumComments  ColumnConfig `yaml:"numComments,omitempty"`
}

type IssuesLayoutConfig struct {
	UpdatedAt   ColumnConfig `yaml:"updatedAt,omitempty"`
	CreatedAt   ColumnConfig `yaml:"createdAt,omitempty"`
	State       ColumnConfig `yaml:"state,omitempty"`
	Repo        ColumnConfig `yaml:"repo,omitempty"`
	Title       ColumnConfig `yaml:"title,omitempty"`
	Creator     ColumnConfig `yaml:"creator,omitempty"`
	CreatorIcon ColumnConfig `yaml:"creatorIcon,omitempty"`
	Assignees   ColumnConfig `yaml:"assignees,omitempty"`
	Comments    ColumnConfig `yaml:"comments,omitempty"`
	Reactions   ColumnConfig `yaml:"reactions,omitempty"`
}

type PrsSectionConfig struct {
	Title   string
	Filters string
	Limit   *int            `yaml:"limit,omitempty"`
	Layout  PrsLayoutConfig `yaml:"layout,omitempty"`
	Type    *ViewType       `yaml:"type,omitempty"`
	// A sift addition: list a repository's merge queue instead of searching.
	MergeQueue *MergeQueueConfig `yaml:"mergeQueue,omitempty"`
}

type MergeQueueConfig struct {
	Repo string
	// Empty means the repository's default branch.
	Branch string `yaml:"branch,omitempty"`
}

type IssuesSectionConfig struct {
	Title   string
	Filters string
	Limit   *int               `yaml:"limit,omitempty"`
	Layout  IssuesLayoutConfig `yaml:"layout,omitempty"`
}

type NotificationsSectionConfig struct {
	Title   string
	Filters string
	Limit   *int `yaml:"limit,omitempty"`
}

type PreviewConfig struct {
	Open     bool
	Width    float64 `yaml:"width"`
	Height   float64 `yaml:"height,omitempty"`
	Position string  `yaml:"position,omitempty"`
}

type LayoutConfig struct {
	Prs    PrsLayoutConfig    `yaml:"prs,omitempty"`
	Issues IssuesLayoutConfig `yaml:"issues,omitempty"`
}

type Defaults struct {
	Preview                PreviewConfig `yaml:"preview"`
	PrsLimit               int           `yaml:"prsLimit"`
	PrApproveComment       string        `yaml:"prApproveComment,omitempty"`
	IssuesLimit            int           `yaml:"issuesLimit"`
	NotificationsLimit     int           `yaml:"notificationsLimit"`
	View                   ViewType      `yaml:"view"`
	Layout                 LayoutConfig  `yaml:"layout,omitempty"`
	RefetchIntervalMinutes int           `yaml:"refetchIntervalMinutes,omitempty"`
	DateFormat             string        `yaml:"dateFormat,omitempty"`
}

type RepoConfig struct {
	BranchesRefetchIntervalSeconds int `yaml:"branchesRefetchIntervalSeconds,omitempty"`
	PrsRefetchIntervalSeconds      int `yaml:"prsRefetchIntervalSeconds,omitempty"`
}

type Keybinding struct {
	Key     string `yaml:"key"`
	Command string `yaml:"command,omitempty"`
	Builtin string `yaml:"builtin,omitempty"`
	Name    string `yaml:"name,omitempty"`
}

type Keybindings struct {
	Universal     []Keybinding `yaml:"universal,omitempty"`
	Issues        []Keybinding `yaml:"issues,omitempty"`
	Prs           []Keybinding `yaml:"prs,omitempty"`
	Branches      []Keybinding `yaml:"branches,omitempty"`
	Notifications []Keybinding `yaml:"notifications,omitempty"`
	Completions   []Keybinding `yaml:"completions,omitempty"`
}

type Pager struct {
	Diff string `yaml:"diff"`
}

type ColorThemeIcon struct {
	NewContributor string `yaml:"newcontributor"`
	Contributor    string `yaml:"contributor"`
	Collaborator   string `yaml:"collaborator"`
	Member         string `yaml:"member"`
	Owner          string `yaml:"owner"`
	UnknownRole    string `yaml:"unknownrole"`
}

type ColorThemeText struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
	Inverted  string `yaml:"inverted"`
	Faint     string `yaml:"faint"`
	Warning   string `yaml:"warning"`
	Success   string `yaml:"success"`
	Error     string `yaml:"error"`
	Actor     string `yaml:"actor"`
}

type ColorThemeBorder struct {
	Primary   string `yaml:"primary"`
	Secondary string `yaml:"secondary"`
	Faint     string `yaml:"faint"`
}

type ColorThemeBackground struct {
	Selected string `yaml:"selected"`
}

type ColorTheme struct {
	Icon       ColorThemeIcon       `yaml:"icon,omitempty"`
	Text       ColorThemeText       `yaml:"text,omitempty"`
	Background ColorThemeBackground `yaml:"background,omitempty"`
	Border     ColorThemeBorder     `yaml:"border,omitempty"`
}

type IconTheme struct {
	NewContributor string `yaml:"newcontributor,omitempty"`
	Contributor    string `yaml:"contributor,omitempty"`
	Collaborator   string `yaml:"collaborator,omitempty"`
	Member         string `yaml:"member,omitempty"`
	Owner          string `yaml:"owner,omitempty"`
	UnknownRole    string `yaml:"unknownrole,omitempty"`
}

type TableUIThemeConfig struct {
	ShowSeparator bool `yaml:"showSeparator"`
	Compact       bool `yaml:"compact"`
}

type UIThemeConfig struct {
	SectionsShowCount bool               `yaml:"sectionsShowCount"`
	Table             TableUIThemeConfig `yaml:"table"`
}

type ThemeConfig struct {
	Ui     UIThemeConfig `yaml:"ui,omitempty"`
	Colors *ColorTheme   `yaml:"colors,omitempty"`
	Icons  *IconTheme    `yaml:"icons,omitempty"`
}

type Config struct {
	Include                  []string                     `yaml:"include,omitempty"`
	PRSections               []PrsSectionConfig           `yaml:"prSections"`
	IssuesSections           []IssuesSectionConfig        `yaml:"issuesSections"`
	NotificationsSections    []NotificationsSectionConfig `yaml:"notificationsSections"`
	Repo                     RepoConfig                   `yaml:"repo,omitempty"`
	Defaults                 Defaults                     `yaml:"defaults"`
	Keybindings              Keybindings                  `yaml:"keybindings"`
	RepoPaths                map[string]string            `yaml:"repoPaths"`
	Theme                    *ThemeConfig                 `yaml:"theme,omitempty"`
	Pager                    Pager                        `yaml:"pager"`
	ConfirmQuit              bool                         `yaml:"confirmQuit"`
	ShowAuthorIcons          bool                         `yaml:"showAuthorIcons,omitempty"`
	SmartFilteringAtLaunch   bool                         `yaml:"smartFilteringAtLaunch"`
	IncludeReadNotifications bool                         `yaml:"includeReadNotifications"`
}

func intPointer(value int) *int    { return &value }
func boolPointer(value bool) *bool { return &value }

func Default() Config {
	return Config{
		Defaults: Defaults{
			Preview:                PreviewConfig{Open: true, Width: 0.45, Height: 0.60, Position: "auto"},
			PrsLimit:               20,
			PrApproveComment:       "LGTM",
			IssuesLimit:            20,
			NotificationsLimit:     20,
			View:                   PRsView,
			RefetchIntervalMinutes: 30,
			Layout: LayoutConfig{
				Prs: PrsLayoutConfig{
					UpdatedAt:  ColumnConfig{Width: intPointer(5)},
					CreatedAt:  ColumnConfig{Width: intPointer(5)},
					Repo:       ColumnConfig{Width: intPointer(20)},
					Author:     ColumnConfig{Width: intPointer(15)},
					AuthorIcon: ColumnConfig{Hidden: boolPointer(false)},
					Labels:     ColumnConfig{Width: intPointer(22), Hidden: boolPointer(true)},
					Assignees:  ColumnConfig{Width: intPointer(20), Hidden: boolPointer(true)},
					Base:       ColumnConfig{Width: intPointer(15), Hidden: boolPointer(true)},
					Lines:      ColumnConfig{Width: intPointer(15)},
				},
				Issues: IssuesLayoutConfig{
					UpdatedAt:   ColumnConfig{Width: intPointer(5)},
					CreatedAt:   ColumnConfig{Width: intPointer(5)},
					Repo:        ColumnConfig{Width: intPointer(15)},
					Creator:     ColumnConfig{Width: intPointer(10)},
					CreatorIcon: ColumnConfig{Hidden: boolPointer(false)},
					Assignees:   ColumnConfig{Width: intPointer(20), Hidden: boolPointer(true)},
				},
			},
		},
		Repo: RepoConfig{BranchesRefetchIntervalSeconds: 30, PrsRefetchIntervalSeconds: 60},
		PRSections: []PrsSectionConfig{
			{Title: "My Pull Requests", Filters: "is:open author:@me"},
			{Title: "Needs My Review", Filters: "is:open review-requested:@me"},
			{Title: "Involved", Filters: "is:open involves:@me -author:@me"},
		},
		IssuesSections: []IssuesSectionConfig{
			{Title: "My Issues", Filters: "is:open author:@me"},
			{Title: "Assigned", Filters: "is:open assignee:@me"},
			{Title: "Involved", Filters: "is:open involves:@me -author:@me"},
		},
		RepoPaths: map[string]string{},
		Theme: &ThemeConfig{
			Ui: UIThemeConfig{SectionsShowCount: true, Table: TableUIThemeConfig{ShowSeparator: true}},
		},
		ShowAuthorIcons:          true,
		SmartFilteringAtLaunch:   true,
		IncludeReadNotifications: true,
	}
}

// GlobalPath is where gh-dash keeps its config; sift reads the same file.
func GlobalPath() string {
	configDirectory := os.Getenv("XDG_CONFIG_HOME")
	if configDirectory == "" {
		homeDirectory, _ := os.UserHomeDir()
		configDirectory = filepath.Join(homeDirectory, ".config")
	}
	return filepath.Join(configDirectory, "gh-dash", "config.yml")
}

// Load layers configs the way gh-dash does: defaults, then the global file,
// then --config / $GH_DASH_CONFIG / <repo>/.gh-dash.yml on top.
func Load(flagPath, repositoryRoot string) (Config, error) {
	config := Default()
	seen := map[string]bool{}
	if _, err := os.Stat(GlobalPath()); err == nil {
		if err := loadLayer(&config, GlobalPath(), seen); err != nil {
			return config, err
		}
	}
	if provided := providedPath(flagPath, repositoryRoot); provided != "" {
		if err := loadLayer(&config, provided, seen); err != nil {
			return config, err
		}
	}
	return config, nil
}

func providedPath(flagPath, repositoryRoot string) string {
	if flagPath != "" {
		return flagPath
	}
	if fromEnvironment := os.Getenv("GH_DASH_CONFIG"); fromEnvironment != "" {
		return fromEnvironment
	}
	if repositoryRoot != "" {
		for _, name := range []string{".gh-dash.yml", ".gh-dash.yaml"} {
			candidate := filepath.Join(repositoryRoot, name)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}
	return ""
}

func loadLayer(config *Config, path string, seen map[string]bool) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	if seen[absolute] {
		return nil
	}
	seen[absolute] = true

	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config %s: %w", path, err)
	}
	var header struct {
		Include []string `yaml:"include"`
	}
	if err := yaml.Unmarshal(contents, &header); err != nil {
		return fmt.Errorf("parsing config %s: %w", path, err)
	}
	// Includes load first so the including file wins.
	for _, include := range header.Include {
		if err := loadLayer(config, resolveInclude(path, include), seen); err != nil {
			return err
		}
	}

	// yaml replaces lists wholesale, which is right for sections but not for
	// keybindings: gh-dash unions those across layers by key.
	previous := config.Keybindings
	config.Keybindings = Keybindings{}
	if err := yaml.Unmarshal(contents, config); err != nil {
		return fmt.Errorf("parsing config %s: %w", path, err)
	}
	config.Keybindings = Keybindings{
		Universal:     unionKeybindings(previous.Universal, config.Keybindings.Universal),
		Issues:        unionKeybindings(previous.Issues, config.Keybindings.Issues),
		Prs:           unionKeybindings(previous.Prs, config.Keybindings.Prs),
		Branches:      unionKeybindings(previous.Branches, config.Keybindings.Branches),
		Notifications: unionKeybindings(previous.Notifications, config.Keybindings.Notifications),
		Completions:   unionKeybindings(previous.Completions, config.Keybindings.Completions),
	}
	return nil
}

func resolveInclude(includingPath, include string) string {
	if strings.HasPrefix(include, "~") {
		homeDirectory, _ := os.UserHomeDir()
		return strings.Replace(include, "~", homeDirectory, 1)
	}
	if filepath.IsAbs(include) {
		return include
	}
	return filepath.Join(filepath.Dir(includingPath), include)
}

func unionKeybindings(lower, upper []Keybinding) []Keybinding {
	merged := []Keybinding{}
	for _, binding := range lower {
		overridden := false
		for _, other := range upper {
			if other.Key == binding.Key {
				overridden = true
			}
		}
		if !overridden {
			merged = append(merged, binding)
		}
	}
	return append(merged, upper...)
}

// RepoPath resolves a repo's local checkout from `repoPaths`: an exact
// "owner/repo" entry wins, then "owner/*", then the ":owner/:repo" template.
func (config Config) RepoPath(nameWithOwner string) (string, bool) {
	owner, name, _ := strings.Cut(nameWithOwner, "/")
	if path, ok := config.RepoPaths[nameWithOwner]; ok {
		return expandHome(path), true
	}
	if path, ok := config.RepoPaths[owner+"/*"]; ok {
		return expandHome(strings.TrimSuffix(path, "*") + name), true
	}
	if template, ok := config.RepoPaths[":owner/:repo"]; ok {
		path := strings.ReplaceAll(template, ":owner", owner)
		return expandHome(strings.ReplaceAll(path, ":repo", name)), true
	}
	return "", false
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~") {
		homeDirectory, _ := os.UserHomeDir()
		return strings.Replace(path, "~", homeDirectory, 1)
	}
	return path
}
