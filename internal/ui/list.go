package ui

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/gh"
)

type section struct {
	title        string
	filters      string
	limit        int
	prLayout     config.PrsLayoutConfig
	issueLayout  config.IssuesLayoutConfig
	pullRequests []gh.PullRequestRow
	issues       []gh.IssueRow
	totalCount   int
	hasNextPage  bool
	endCursor    string
	loading      bool
	loadedAt     time.Time
	err          error
	cursor       int
	top          int
	// Bumped on every refetch so a slow, superseded response is dropped.
	generation int
}

type listScreen struct {
	kind          config.ViewType
	prSections    []*section
	issueSections []*section
	prIndex       int
	issueIndex    int
	previewOpen   bool
	// Set by P: the preview on the other side from where the config puts it.
	flipPreview    bool
	previewScroll  int
	previewRequest int
	smartFiltering bool
	searching      bool
	search         textinput.Model
	issuesFetched  bool
	// Last results by view and query, mirrored to disk.
	cache map[string]cachedSection
}

type sectionLoadedMsg struct {
	view         config.ViewType
	index        int
	generation   int
	appended     bool
	pullRequests gh.Page[gh.PullRequestRow]
	issues       gh.Page[gh.IssueRow]
	err          error
}

// previewMsg fires a moment after the cursor settles on a row.
type previewMsg struct {
	key     pullRequestKey
	request int
}

func mergeColumn(base, override config.ColumnConfig) config.ColumnConfig {
	if override.Width != nil {
		base.Width = override.Width
	}
	if override.Hidden != nil {
		base.Hidden = override.Hidden
	}
	return base
}

func newListScreen(app *App) listScreen {
	defaults := app.config.Defaults
	list := listScreen{
		kind:           defaults.View,
		previewOpen:    defaults.Preview.Open,
		smartFiltering: app.config.SmartFilteringAtLaunch,
	}
	if list.kind != config.IssuesView {
		list.kind = config.PRsView
	}
	for _, configured := range app.config.PRSections {
		layout, override := defaults.Layout.Prs, configured.Layout
		layout.UpdatedAt = mergeColumn(layout.UpdatedAt, override.UpdatedAt)
		layout.CreatedAt = mergeColumn(layout.CreatedAt, override.CreatedAt)
		layout.Repo = mergeColumn(layout.Repo, override.Repo)
		layout.Author = mergeColumn(layout.Author, override.Author)
		layout.Base = mergeColumn(layout.Base, override.Base)
		layout.ReviewStatus = mergeColumn(layout.ReviewStatus, override.ReviewStatus)
		layout.State = mergeColumn(layout.State, override.State)
		layout.Ci = mergeColumn(layout.Ci, override.Ci)
		layout.Lines = mergeColumn(layout.Lines, override.Lines)
		layout.NumComments = mergeColumn(layout.NumComments, override.NumComments)
		limit := defaults.PrsLimit
		if configured.Limit != nil {
			limit = *configured.Limit
		}
		list.prSections = append(list.prSections, &section{title: configured.Title, filters: configured.Filters, limit: limit, prLayout: layout})
	}
	for _, configured := range app.config.IssuesSections {
		layout, override := defaults.Layout.Issues, configured.Layout
		layout.UpdatedAt = mergeColumn(layout.UpdatedAt, override.UpdatedAt)
		layout.CreatedAt = mergeColumn(layout.CreatedAt, override.CreatedAt)
		layout.Repo = mergeColumn(layout.Repo, override.Repo)
		layout.Creator = mergeColumn(layout.Creator, override.Creator)
		layout.Comments = mergeColumn(layout.Comments, override.Comments)
		layout.Reactions = mergeColumn(layout.Reactions, override.Reactions)
		limit := defaults.IssuesLimit
		if configured.Limit != nil {
			limit = *configured.Limit
		}
		list.issueSections = append(list.issueSections, &section{title: configured.Title, filters: configured.Filters, limit: limit, issueLayout: layout})
	}
	list.cache = loadSectionCache()
	for _, current := range list.prSections {
		cached := list.cache[string(config.PRsView)+"|"+list.query(app, current)]
		current.pullRequests, current.totalCount = cached.PullRequests, cached.TotalCount
	}
	for _, current := range list.issueSections {
		cached := list.cache[string(config.IssuesView)+"|"+list.query(app, current)]
		current.issues, current.totalCount = cached.Issues, cached.TotalCount
	}
	return list
}

func (list *listScreen) sections() []*section {
	if list.kind == config.IssuesView {
		return list.issueSections
	}
	return list.prSections
}

func (list *listScreen) index() *int {
	if list.kind == config.IssuesView {
		return &list.issueIndex
	}
	return &list.prIndex
}

func (list *listScreen) current() *section {
	sections := list.sections()
	if len(sections) == 0 {
		return &section{}
	}
	return sections[*list.index()]
}

func (current *section) count() int {
	return len(current.pullRequests) + len(current.issues)
}

func (current *section) selectedPullRequest() *gh.PullRequestRow {
	if current.cursor < len(current.pullRequests) {
		return &current.pullRequests[current.cursor]
	}
	return nil
}

func (current *section) selectedIssue() *gh.IssueRow {
	if current.cursor < len(current.issues) {
		return &current.issues[current.cursor]
	}
	return nil
}

// query is what goes to GitHub: the filters with their template rendered
// and, under smart filtering, narrowed to the repository sift started in.
func (list *listScreen) query(app *App, current *section) string {
	filters := current.filters
	functions := template.FuncMap{"nowModify": func(offset string) string {
		// gh-dash documents filters like `updated:>={{ nowModify "-3w" }}`.
		unit := map[byte]time.Duration{'d': 24 * time.Hour, 'w': 7 * 24 * time.Hour, 'y': 365 * 24 * time.Hour}[offset[len(offset)-1]]
		if unit != 0 {
			count, _ := strconv.Atoi(offset[:len(offset)-1])
			return time.Now().Add(time.Duration(count) * unit).Format("2006-01-02")
		}
		parsed, _ := time.ParseDuration(offset)
		return time.Now().Add(parsed).Format("2006-01-02")
	}}
	if parsed, err := template.New("filters").Funcs(functions).Parse(filters); err == nil {
		var rendered bytes.Buffer
		if parsed.Execute(&rendered, map[string]any{"Now": time.Now()}) == nil {
			filters = rendered.String()
		}
	}
	if prefix := list.repositoryFilter(app, filters); prefix != "" {
		filters = prefix + " " + filters
	}
	return filters
}

func (list *listScreen) repositoryFilter(app *App, filters string) string {
	if list.smartFiltering && app.repository != "" && !strings.Contains(filters, "repo:") {
		return "repo:" + app.repository
	}
	return ""
}

func (list *listScreen) fetch(app *App, view config.ViewType, index int, more bool) tea.Cmd {
	sections := list.prSections
	if view == config.IssuesView {
		sections = list.issueSections
	}
	current := sections[index]
	if !more {
		current.generation++
	}
	current.loading = true
	generation, after := current.generation, ""
	if more {
		after = current.endCursor
	}
	query, limit, client := list.query(app, current), current.limit, app.client
	return func() tea.Msg {
		message := sectionLoadedMsg{view: view, index: index, generation: generation, appended: more}
		if view == config.IssuesView {
			message.issues, message.err = client.SearchIssues(query, limit, after)
		} else {
			message.pullRequests, message.err = client.SearchPullRequests(query, limit, after)
		}
		return message
	}
}

func (list *listScreen) fetchAll(app *App) []tea.Cmd {
	var commands []tea.Cmd
	for index := range list.sections() {
		commands = append(commands, list.fetch(app, list.kind, index, false))
	}
	if list.kind == config.IssuesView {
		list.issuesFetched = true
	}
	return commands
}

func (list *listScreen) refreshCurrent(app *App) tea.Cmd {
	if len(list.sections()) == 0 {
		return nil
	}
	return list.fetch(app, list.kind, *list.index(), false)
}

func (list *listScreen) periodic(app *App) []tea.Cmd {
	interval := time.Duration(app.config.Defaults.RefetchIntervalMinutes) * time.Minute
	if interval == 0 || app.screen != screenList {
		return nil
	}
	var commands []tea.Cmd
	for index, current := range list.sections() {
		if !current.loading && !current.loadedAt.IsZero() && time.Since(current.loadedAt) > interval {
			commands = append(commands, list.fetch(app, list.kind, index, false))
		}
	}
	return commands
}

func (list *listScreen) loaded(app *App, message sectionLoadedMsg) tea.Cmd {
	sections := list.prSections
	if message.view == config.IssuesView {
		sections = list.issueSections
	}
	current := sections[message.index]
	if message.generation != current.generation {
		return nil
	}
	current.loading = false
	current.err = message.err
	if message.err != nil {
		return nil
	}
	current.loadedAt = time.Now()
	if message.view == config.IssuesView {
		page := message.issues
		if !message.appended {
			current.issues = nil
		}
		current.issues = append(current.issues, page.Rows...)
		current.totalCount, current.hasNextPage, current.endCursor = page.TotalCount, page.HasNextPage, page.EndCursor
	} else {
		page := message.pullRequests
		if !message.appended {
			current.pullRequests = nil
		}
		current.pullRequests = append(current.pullRequests, page.Rows...)
		current.totalCount, current.hasNextPage, current.endCursor = page.TotalCount, page.HasNextPage, page.EndCursor
	}
	current.cursor = clamp(current.cursor, 0, max(current.count()-1, 0))
	if !message.appended {
		list.cache[string(message.view)+"|"+list.query(app, current)] = cachedSection{TotalCount: current.totalCount, PullRequests: current.pullRequests, Issues: current.issues}
		saveSectionCache(list.cache)
	}
	if current == list.current() {
		return list.schedulePreview(app)
	}
	return nil
}

// schedulePreview waits a beat before fetching the selected PR, so holding
// j does not fetch every row on the way.
func (list *listScreen) schedulePreview(app *App) tea.Cmd {
	list.previewScroll = 0
	row := list.current().selectedPullRequest()
	if row == nil || !list.previewOpen || list.kind != config.PRsView {
		return nil
	}
	list.previewRequest++
	message := previewMsg{key: pullRequestKey{row.Repository, row.Number}, request: list.previewRequest}
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return message })
}

func (list *listScreen) previewDue(app *App, message previewMsg) tea.Cmd {
	if message.request != list.previewRequest || app.screen != screenList {
		return nil
	}
	return app.loadPullRequest(message.key, false)
}

// updateRow copies fresh detail into matching list rows, so the list agrees
// with what the PR view just showed without a refetch.
func (list *listScreen) updateRow(detail *gh.PullRequest) {
	for _, current := range list.prSections {
		for index := range current.pullRequests {
			if row := &current.pullRequests[index]; row.Repository == detail.Repository && row.Number == detail.Number {
				stack := row.Stack
				*row = detail.PullRequestRow
				// The row's own stack entries are enough for the list.
				if detail.Stack == nil {
					row.Stack = stack
				}
			}
		}
	}
}

func (list *listScreen) move(app *App, delta int) tea.Cmd {
	current := list.current()
	target := clamp(current.cursor+delta, 0, max(current.count()-1, 0))
	var commands []tea.Cmd
	if target == current.count()-1 && current.hasNextPage && !current.loading {
		commands = append(commands, list.fetch(app, list.kind, *list.index(), true))
	}
	if target != current.cursor {
		current.cursor = target
		commands = append(commands, list.schedulePreview(app))
	}
	return tea.Batch(commands...)
}

func (list *listScreen) switchSection(app *App, delta int) tea.Cmd {
	count := len(list.sections())
	if count == 0 {
		return nil
	}
	*list.index() = (*list.index() + delta + count) % count
	return list.schedulePreview(app)
}

func (list *listScreen) switchView(app *App) tea.Cmd {
	if list.kind == config.PRsView {
		list.kind = config.IssuesView
	} else {
		list.kind = config.PRsView
	}
	if list.kind == config.IssuesView && !list.issuesFetched {
		return tea.Batch(list.fetchAll(app)...)
	}
	return list.schedulePreview(app)
}

func (list *listScreen) startSearch(app *App) {
	list.searching = true
	list.search = textinput.New()
	list.search.Prompt = ""
	list.search.SetValue(list.current().filters)
	list.search.CursorEnd()
	list.search.Focus()
}

func (list *listScreen) updateSearch(app *App, message tea.Msg) tea.Cmd {
	if key, ok := message.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc", "ctrl+c":
			list.searching = false
			return nil
		case "enter":
			list.searching = false
			current := list.current()
			current.filters = list.search.Value()
			current.cursor, current.top = 0, 0
			return list.refreshCurrent(app)
		}
	}
	var command tea.Cmd
	list.search, command = list.search.Update(message)
	return command
}

func (list *listScreen) handleKey(app *App, pressed string) tea.Cmd {
	current := list.current()
	pullRequest, issue := current.selectedPullRequest(), current.selectedIssue()

	// User keybindings come first, so they can shadow a builtin as in gh-dash.
	if list.kind == config.PRsView && pullRequest != nil {
		if custom := matchCustom(app.keys.customPRs, pressed); custom != nil {
			return app.runCustomCommand(custom.Command, map[string]any{
				"RepoName": pullRequest.Repository, "PrNumber": pullRequest.Number,
				"HeadRefName": pullRequest.HeadRefName, "BaseRefName": pullRequest.BaseRefName, "Author": pullRequest.Author,
			})
		}
	}
	if list.kind == config.IssuesView && issue != nil {
		if custom := matchCustom(app.keys.customIssues, pressed); custom != nil {
			return app.runCustomCommand(custom.Command, map[string]any{
				"RepoName": issue.Repository, "IssueNumber": issue.Number, "IssueTitle": issue.Title, "Author": issue.Author,
			})
		}
	}
	if custom := matchCustom(app.keys.customUniversal, pressed); custom != nil {
		return app.runCustomCommand(custom.Command, map[string]any{})
	}

	url, number := "", 0
	if pullRequest != nil {
		url, number = pullRequest.URL, pullRequest.Number
	} else if issue != nil {
		url, number = issue.URL, issue.Number
	}

	switch match(app.keys.universal, pressed) {
	case "up":
		return list.move(app, -1)
	case "down":
		return list.move(app, 1)
	case "firstLine":
		return list.move(app, -current.count())
	case "lastLine":
		return list.move(app, current.count())
	case "prevSection":
		return list.switchSection(app, -1)
	case "nextSection":
		return list.switchSection(app, 1)
	case "pageDown":
		list.previewScroll += max(app.height/2, 1)
		return nil
	case "pageUp":
		list.previewScroll = max(list.previewScroll-app.height/2, 0)
		return nil
	case "togglePreviewPosition":
		list.flipPreview = !list.flipPreview
		return nil
	case "togglePreview":
		list.previewOpen = !list.previewOpen
		return list.schedulePreview(app)
	case "openGithub":
		if url != "" {
			return app.openInBrowser(url)
		}
		return nil
	case "refresh":
		return list.refreshCurrent(app)
	case "refreshAll":
		return tea.Batch(list.fetchAll(app)...)
	case "search":
		list.startSearch(app)
		return nil
	case "copyNumber":
		if number > 0 {
			return app.copy(strconv.Itoa(number), fmt.Sprintf("#%d", number))
		}
		return nil
	case "copyurl":
		if url != "" {
			return app.copy(url, url)
		}
		return nil
	case "help":
		app.showHelp = true
		return nil
	case "quit":
		if app.config.ConfirmQuit {
			app.openConfirm("Really quit?", func(*App) tea.Cmd { return tea.Quit })
			return nil
		}
		return tea.Quit
	}

	if list.kind == config.IssuesView {
		return list.handleIssueKey(app, match(app.keys.issues, pressed), issue)
	}
	return list.handlePullRequestKey(app, match(app.keys.prs, pressed), pullRequest)
}

func (list *listScreen) handlePullRequestKey(app *App, action string, row *gh.PullRequestRow) tea.Cmd {
	switch action {
	case "toggleSmartFiltering":
		return list.toggleSmartFiltering(app)
	case "viewIssues":
		return list.switchView(app)
	}
	if row == nil || action == "" {
		return nil
	}
	key := pullRequestKey{row.Repository, row.Number}
	repository, number := row.Repository, strconv.Itoa(row.Number)
	refresh := func(app *App) tea.Cmd { return app.list.refreshCurrent(app) }
	// gh-dash's simple actions are one `gh pr` call behind a y/N question.
	confirmed := func(question, doing, done string, arguments ...string) tea.Cmd {
		app.openConfirm(question, func(app *App) tea.Cmd {
			return app.run(doing, func() error { return app.client.PullRequestCommand(repository, "", arguments...) }, done, refresh)
		})
		return nil
	}

	switch action {
	case "open":
		return app.openPullRequest(key, row, tabOverview)
	case "diff":
		return app.openPullRequest(key, row, tabFiles)
	case "stackUp", "stackDown":
		delta := map[string]int{"stackUp": 1, "stackDown": -1}[action]
		if neighbour := row.Stack.Neighbour(row.StackPosition, delta); neighbour != nil {
			current := list.current()
			for index, candidate := range current.pullRequests {
				if candidate.Repository == row.Repository && candidate.Number == neighbour.Number {
					return list.move(app, index-current.cursor)
				}
			}
		}
		// Not a row of this section: open it instead of doing nothing.
		return app.moveInStack(row.Repository, row.Stack, row.StackPosition, delta, tabOverview)
	case "merge":
		return app.startMerge(key)
	case "approve":
		app.openInput(fmt.Sprintf("Approve #%d", row.Number), app.config.Defaults.PrApproveComment, func(app *App, body string) tea.Cmd {
			pending := ""
			if detail := app.data(key).detail; detail != nil {
				pending = detail.PendingReviewID
			}
			return app.run("approving", func() error { return app.client.SubmitReview(row.ID, pending, "APPROVE", body) },
				fmt.Sprintf("approved #%d", row.Number), refresh)
		})
	case "comment":
		app.openComposer(fmt.Sprintf("Comment on #%d", row.Number), "", func(app *App, body string) tea.Cmd {
			return app.run("commenting", func() error { return app.client.Comment(repository, row.Number, body) },
				fmt.Sprintf("commented on #%d", row.Number), refresh)
		})
	case "assign", "unassign":
		flag := map[string]string{"assign": "--add-assignee", "unassign": "--remove-assignee"}[action]
		initial := ""
		if action == "unassign" {
			initial = strings.Join(row.Assignees, " ")
		}
		app.openInput(fmt.Sprintf("%s #%d (logins, space separated)", strings.ToUpper(action[:1])+action[1:], row.Number), initial, func(app *App, value string) tea.Cmd {
			logins := strings.Join(strings.Fields(value), ",")
			if logins == "" {
				return nil
			}
			return app.run(action+"ing", func() error { return app.client.PullRequestCommand(repository, "", "edit", number, flag, logins) },
				action+"ed "+logins, refresh)
		})
	case "label":
		var existing []string
		for _, label := range row.Labels {
			existing = append(existing, label.Name)
		}
		app.openInput(fmt.Sprintf("Labels of #%d (comma separated)", row.Number), strings.Join(existing, ", "), func(app *App, value string) tea.Cmd {
			arguments := []string{"edit", number}
			var wanted []string
			for _, label := range strings.Split(value, ",") {
				if label = strings.TrimSpace(label); label != "" {
					wanted = append(wanted, label)
					if !slices.Contains(existing, label) {
						arguments = append(arguments, "--add-label", label)
					}
				}
			}
			for _, label := range existing {
				if !slices.Contains(wanted, label) {
					arguments = append(arguments, "--remove-label", label)
				}
			}
			if len(arguments) == 2 {
				return nil
			}
			return app.run("labelling", func() error { return app.client.PullRequestCommand(repository, "", arguments...) }, "labels updated", refresh)
		})
	case "checkout":
		path, found := app.config.RepoPath(repository)
		if !found {
			app.notify("no local path for "+repository+": add it to repoPaths in the config", true)
			return nil
		}
		return app.run("checking out", func() error { return app.client.PullRequestCommand(repository, path, "checkout", number) },
			fmt.Sprintf("checked out #%d in %s", row.Number, path), nil)
	case "close":
		return confirmed("Are you sure you want to close this PR? (y/N)", "closing", "closed #"+number, "close", number)
	case "reopen":
		return confirmed("Are you sure you want to reopen this PR? (y/N)", "reopening", "reopened #"+number, "reopen", number)
	case "ready":
		return confirmed("Are you sure you want to mark this PR as ready? (y/N)", "marking ready", "#"+number+" is ready for review", "ready", number)
	case "update":
		return confirmed("Are you sure you want to update this PR from its base? (y/N)", "updating", "updated #"+number, "update-branch", number)
	default:
		app.notify(action+" is not supported yet", false)
	}
	return nil
}

func (list *listScreen) handleIssueKey(app *App, action string, row *gh.IssueRow) tea.Cmd {
	switch action {
	case "toggleSmartFiltering":
		return list.toggleSmartFiltering(app)
	case "viewPrs":
		return list.switchView(app)
	}
	if row == nil || action == "" {
		return nil
	}
	repository, number := row.Repository, strconv.Itoa(row.Number)
	refresh := func(app *App) tea.Cmd { return app.list.refreshCurrent(app) }
	switch action {
	case "comment":
		app.openComposer(fmt.Sprintf("Comment on #%d", row.Number), "", func(app *App, body string) tea.Cmd {
			return app.run("commenting", func() error { return app.client.Comment(repository, row.Number, body) }, "commented on #"+number, refresh)
		})
	case "close", "reopen":
		app.openConfirm(fmt.Sprintf("Are you sure you want to %s this issue? (y/N)", action), func(app *App) tea.Cmd {
			return app.run(action, func() error { return app.client.IssueCommand(repository, action, number) }, action+" #"+number+" done", refresh)
		})
	case "assign", "unassign":
		flag := map[string]string{"assign": "--add-assignee", "unassign": "--remove-assignee"}[action]
		app.openInput(fmt.Sprintf("%s #%d (logins, space separated)", action, row.Number), "", func(app *App, value string) tea.Cmd {
			logins := strings.Join(strings.Fields(value), ",")
			if logins == "" {
				return nil
			}
			return app.run(action+"ing", func() error { return app.client.IssueCommand(repository, "edit", number, flag, logins) }, action+"ed "+logins, refresh)
		})
	case "label":
		app.openInput(fmt.Sprintf("Add labels to #%d (comma separated)", row.Number), "", func(app *App, value string) tea.Cmd {
			if value == "" {
				return nil
			}
			return app.run("labelling", func() error { return app.client.IssueCommand(repository, "edit", number, "--add-label", value) }, "labels updated", refresh)
		})
	default:
		app.notify(action+" is not supported yet", false)
	}
	return nil
}

func (list *listScreen) toggleSmartFiltering(app *App) tea.Cmd {
	if app.repository == "" {
		app.notify("smart filtering needs sift to be started inside a repository", false)
		return nil
	}
	list.smartFiltering = !list.smartFiltering
	return tea.Batch(list.fetchAll(app)...)
}
