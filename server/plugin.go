package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	pluginID         = "com.cw.remind"
	indexKey         = "reminder_index"
	reminderPostType = "custom_cw_reminder"
)

type Plugin struct {
	plugin.MattermostPlugin
	botUserID string
	stop      chan struct{}
	storeLock sync.Mutex
}

func (p *Plugin) OnActivate() error {
	botUserID, err := p.API.EnsureBotUser(&model.Bot{
		Username:    "duewatch",
		DisplayName: "DueWatch",
		Description: "Posts scheduled reminders created with /remind.",
		OwnerId:     pluginID,
	})
	if err != nil {
		return fmt.Errorf("ensure reminder bot: %w", err)
	}
	p.botUserID = botUserID
	if err := p.setBotIcon(); err != nil {
		return err
	}

	if err := p.API.RegisterCommand(&model.Command{Trigger: "remind", AutoComplete: true, AutoCompleteDesc: "期限リマインダーを設定 / 设置期限提醒", DisplayName: "CW Remind"}); err != nil {
		return fmt.Errorf("register /remind: %w", err)
	}
	p.stop = make(chan struct{})
	go p.runScheduler()
	go p.migrateReminderPosts()
	return nil
}

func (p *Plugin) setBotIcon() error {
	bundlePath, err := p.API.GetBundlePath()
	if err != nil {
		return fmt.Errorf("get plugin bundle path: %w", err)
	}
	icon, err := os.ReadFile(filepath.Join(bundlePath, "assets", "duewatch-icon.png"))
	if err != nil {
		return fmt.Errorf("read DueWatch bot icon: %w", err)
	}
	if appErr := p.API.SetProfileImage(p.botUserID, icon); appErr != nil {
		return fmt.Errorf("set DueWatch bot icon: %s", appErr.Error())
	}
	return nil
}

func (p *Plugin) OnDeactivate() error {
	if p.stop != nil {
		close(p.stop)
	}
	return nil
}

func (p *Plugin) ExecuteCommand(_ *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	if strings.TrimSpace(args.Command) != "/remind" {
		return &model.CommandResponse{ResponseType: model.CommandResponseTypeEphemeral, Text: "用法: /remind"}, nil
	}
	p.API.PublishWebSocketEvent("open", map[string]interface{}{"channel_id": args.ChannelId}, &model.WebsocketBroadcast{UserId: args.UserId})
	return &model.CommandResponse{ResponseType: model.CommandResponseTypeEphemeral, Text: ""}, nil
}

func (p *Plugin) ServeHTTP(_ *plugin.Context, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/v1/actions/complete" && r.Method == http.MethodPost {
		p.handleStatusAction(w, r, "completed")
		return
	}
	if r.URL.Path == "/api/v1/actions/acknowledge" && r.Method == http.MethodPost {
		p.handleStatusAction(w, r, "acknowledged")
		return
	}
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	switch {
	case path == "api/v1/reminders" && r.Method == http.MethodPost:
		p.handleCreateReminder(w, r, userID)
	case path == "api/v1/reminders" && r.Method == http.MethodGet:
		p.handleListReminders(w, r, userID)
	case path == "api/v1/channel-users" && r.Method == http.MethodGet:
		p.handleChannelUsers(w, r, userID)
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "reminders" && r.Method == http.MethodGet:
		p.handleGetReminder(w, userID, parts[3])
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "reminders" && r.Method == http.MethodPut:
		p.handleUpdateReminder(w, r, userID, parts[3])
	case len(parts) == 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "reminders" && r.Method == http.MethodDelete:
		p.handleDeleteReminder(w, r, userID, parts[3])
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "reminders" && parts[4] == "acknowledge" && r.Method == http.MethodPost:
		p.handleAuthenticatedStatus(w, userID, parts[3], "acknowledged")
	case len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "reminders" && parts[4] == "complete" && r.Method == http.MethodPost:
		p.handleAuthenticatedStatus(w, userID, parts[3], "completed")
	default:
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}
}

func (p *Plugin) handleGetReminder(w http.ResponseWriter, userID, reminderID string) {
	reminder, err := p.getReminder(reminderID)
	if err != nil {
		writeError(w, http.StatusNotFound, "reminder not found")
		return
	}
	if err := p.requireChannelMember(reminder.ChannelID, userID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(publicReminder(reminder))
}

func (p *Plugin) handleAuthenticatedStatus(w http.ResponseWriter, userID, reminderID, status string) {
	current, err := p.getReminder(reminderID)
	if err != nil {
		writeError(w, http.StatusNotFound, "reminder not found")
		return
	}
	if err := p.requireChannelMember(current.ChannelID, userID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	reminder, _, err := p.setUserStatus(reminderID, current.ActionToken, userID, current.ChannelID, status)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.updateAllReminderPosts(reminder)
	_ = json.NewEncoder(w).Encode(publicReminder(reminder))
}

func (p *Plugin) handleCreateReminder(w http.ResponseWriter, r *http.Request, userID string) {
	var req createReminderRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	reminder, err := p.validateAndBuild(userID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	post, err := p.createReminderPost(reminder, true)
	if err != nil {
		p.API.LogError("announce reminder", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "could not announce reminder")
		return
	}
	reminder.AnnouncementPostID = post.Id
	reminder.Posts = []ReminderPost{{ID: post.Id, Announcement: true}}
	if err := p.saveReminder(reminder); err != nil {
		if appErr := p.API.DeletePost(post.Id); appErr != nil {
			p.API.LogError("delete orphan reminder post", "post_id", post.Id, "error", appErr.Error())
		}
		p.API.LogError("save reminder", "error", err.Error())
		writeError(w, 500, "could not save reminder")
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(publicReminder(reminder))
}

func (p *Plugin) handleUpdateReminder(w http.ResponseWriter, r *http.Request, userID, reminderID string) {
	existing, err := p.getReminder(reminderID)
	if err != nil {
		writeError(w, http.StatusNotFound, "reminder not found")
		return
	}
	if existing.CreatorID != userID {
		writeError(w, http.StatusForbidden, "只有创建者可以编辑提醒")
		return
	}
	if existing.DeletedAt != 0 {
		writeError(w, http.StatusConflict, "已删除的提醒不能编辑")
		return
	}
	var req createReminderRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.ChannelID != existing.ChannelID {
		writeError(w, http.StatusBadRequest, "不能更改提醒所属频道")
		return
	}
	updated, err := p.validateAndBuild(userID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated.ID = existing.ID
	updated.CreatorID = existing.CreatorID
	updated.CreatorUsername = existing.CreatorUsername
	updated.CreatedAt = existing.CreatedAt
	updated.ActionToken = existing.ActionToken
	if updated.ActionToken == "" {
		updated.ActionToken = model.NewId()
	}
	updated.AnnouncementPostID = existing.AnnouncementPostID
	updated.Posts = reminderPostRecords(existing)
	updated.UpdatedAt = time.Now().UnixMilli()
	retainUserStatuses(existing, updated)
	if err := p.replaceReminder(updated); err != nil {
		p.API.LogError("update reminder", "id", reminderID, "error", err.Error())
		writeError(w, http.StatusInternalServerError, "could not update reminder")
		return
	}
	p.updateAllReminderPosts(updated)
	_ = json.NewEncoder(w).Encode(publicReminder(updated))
}

func (p *Plugin) handleDeleteReminder(w http.ResponseWriter, _ *http.Request, userID, reminderID string) {
	reminder, err := p.getReminder(reminderID)
	if err != nil {
		writeError(w, http.StatusNotFound, "reminder not found")
		return
	}
	if reminder.CreatorID != userID {
		writeError(w, http.StatusForbidden, "只有创建者可以删除提醒")
		return
	}
	if reminder.DeletedAt == 0 {
		reminder.DeletedAt = time.Now().UnixMilli()
		reminder.DeletedBy = userID
		if err := p.replaceReminder(reminder); err != nil {
			p.API.LogError("delete reminder", "id", reminderID, "error", err.Error())
			writeError(w, http.StatusInternalServerError, "could not delete reminder")
			return
		}
		p.updateAllReminderPosts(reminder)
	}
	_ = json.NewEncoder(w).Encode(publicReminder(reminder))
}

func retainUserStatuses(existing, updated *Reminder) {
	targets := make(map[string]bool, len(updated.TargetUsers))
	for _, target := range updated.TargetUsers {
		targets[target.ID] = true
	}
	for _, acknowledgement := range existing.Acknowledgements {
		if targets[acknowledgement.UserID] {
			updated.Acknowledgements = append(updated.Acknowledgements, acknowledgement)
		}
	}
	for _, completion := range existing.Completions {
		if targets[completion.UserID] {
			updated.Completions = append(updated.Completions, completion)
		}
	}
}

func (p *Plugin) handleListReminders(w http.ResponseWriter, r *http.Request, userID string) {
	channelID := r.URL.Query().Get("channel_id")
	if err := p.requireChannelMember(channelID, userID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	reminders, err := p.listReminders(channelID)
	if err != nil {
		p.API.LogError("list reminders", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "could not load reminders")
		return
	}
	for i := range reminders {
		reminders[i] = *publicReminder(&reminders[i])
	}
	_ = json.NewEncoder(w).Encode(reminders)
}

func publicReminder(reminder *Reminder) *Reminder {
	copy := *reminder
	copy.ActionToken = ""
	return &copy
}

func (p *Plugin) handleStatusAction(w http.ResponseWriter, r *http.Request, status string) {
	var action model.PostActionIntegrationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	if err := decoder.Decode(&action); err != nil {
		writeActionError(w, "確認リクエストを読み取れませんでした。")
		return
	}
	if headerUserID := r.Header.Get("Mattermost-User-ID"); headerUserID != "" && headerUserID != action.UserId {
		writeActionError(w, "ユーザー情報を確認できませんでした。")
		return
	}
	reminderID, _ := action.Context["reminder_id"].(string)
	token, _ := action.Context["token"].(string)
	if reminderID == "" || token == "" || action.UserId == "" || action.PostId == "" {
		writeActionError(w, "確認リクエストが無効です。")
		return
	}
	post, appErr := p.API.GetPost(action.PostId)
	if appErr != nil || post.UserId != p.botUserID || post.ChannelId != action.ChannelId {
		writeActionError(w, "この投稿を確認できませんでした。")
		return
	}
	reminder, unchanged, err := p.setUserStatus(reminderID, token, action.UserId, action.ChannelId, status)
	if err != nil {
		writeActionError(w, err.Error())
		return
	}
	p.setReminderAttachment(post, reminder)
	for _, reminderPost := range reminderPostRecords(reminder) {
		if reminderPost.ID != post.Id {
			p.updateReminderPost(reminderPost, reminder)
		}
	}
	message := "対応済みとして記録しました。"
	if status == "acknowledged" {
		message = "了解として記録しました。"
	}
	if unchanged {
		message = "状態はすでに記録されています。"
	}
	_ = json.NewEncoder(w).Encode(&model.PostActionIntegrationResponse{
		Update:           post,
		EphemeralText:    message,
		SkipSlackParsing: true,
	})
}

func writeActionError(w http.ResponseWriter, message string) {
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": message}})
}

func (p *Plugin) handleChannelUsers(w http.ResponseWriter, r *http.Request, userID string) {
	channelID := r.URL.Query().Get("channel_id")
	if err := p.requireChannelMember(channelID, userID); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	var users []*model.User
	var appErr *model.AppError
	if query == "" {
		users, appErr = p.API.GetUsersInChannel(channelID, "username", 0, 100)
	} else {
		users, appErr = p.API.SearchUsers(&model.UserSearch{Term: query, InChannelId: channelID, Limit: 50})
	}
	if appErr != nil {
		p.API.LogError("search channel users", "error", appErr.Error())
		writeError(w, http.StatusInternalServerError, "could not load channel users")
		return
	}
	result := make([]channelUser, 0, len(users))
	for _, user := range users {
		if user.DeleteAt != 0 || user.IsBot {
			continue
		}
		displayName := strings.TrimSpace(user.FirstName + " " + user.LastName)
		if displayName == "" {
			displayName = user.Username
		}
		result = append(result, channelUser{ID: user.Id, Username: user.Username, DisplayName: displayName})
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (p *Plugin) requireChannelMember(channelID, userID string) error {
	if channelID == "" {
		return fmt.Errorf("频道不能为空")
	}
	if _, appErr := p.API.GetChannelMember(channelID, userID); appErr != nil {
		return fmt.Errorf("你不是该频道成员")
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (p *Plugin) validateAndBuild(userID string, req createReminderRequest) (*Reminder, error) {
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || len([]rune(req.Title)) > 200 {
		return nil, fmt.Errorf("提醒标题必须为 1–200 字符")
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" || len([]rune(req.Content)) > 4000 {
		return nil, fmt.Errorf("提醒内容必须为 1–4000 字符")
	}
	if err := p.requireChannelMember(req.ChannelID, userID); err != nil {
		return nil, err
	}
	if req.Audience != "all" && req.Audience != "channel" && req.Audience != "users" {
		return nil, fmt.Errorf("提醒对象无效")
	}
	user, appErr := p.API.GetUser(userID)
	if appErr != nil {
		return nil, fmt.Errorf("无法读取建立者")
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	loc, err := time.LoadLocation(req.Timezone)
	if err != nil {
		return nil, fmt.Errorf("时区无效")
	}
	due, err := time.ParseInLocation("2006-01-02", req.DueDate, loc)
	if err != nil {
		return nil, fmt.Errorf("期限日期无效")
	}
	if len(req.Rules) == 0 || len(req.Rules) > 50 {
		return nil, fmt.Errorf("提醒规则必须为 1–50 组")
	}
	seen := map[string]bool{}
	for i := range req.Rules {
		rule := &req.Rules[i]
		if rule.DaysBefore < 0 || rule.DaysBefore > 3650 {
			return nil, fmt.Errorf("提前天数必须在 0–3650 之间")
		}
		clock, e := time.Parse("15:04", rule.Time)
		if e != nil {
			return nil, fmt.Errorf("提醒时间无效")
		}
		key := fmt.Sprintf("%d/%s", rule.DaysBefore, rule.Time)
		if seen[key] {
			return nil, fmt.Errorf("提醒规则重复")
		}
		seen[key] = true
		fire := time.Date(due.Year(), due.Month(), due.Day(), clock.Hour(), clock.Minute(), 0, 0, loc).AddDate(0, 0, -rule.DaysBefore)
		if !fire.After(time.Now()) {
			return nil, fmt.Errorf("提醒时点已过期")
		}
		rule.FireAt = fire.UnixMilli()
		rule.FiredAt = 0
	}
	usernames := make([]string, 0, len(req.Usernames))
	targetUsers := make([]ReminderUser, 0)
	if req.Audience == "users" {
		if len(req.Usernames) == 0 {
			return nil, fmt.Errorf("请至少指定一个用户")
		}
		uniq := map[string]bool{}
		for _, raw := range req.Usernames {
			name := strings.TrimPrefix(strings.TrimSpace(raw), "@")
			if name == "" || uniq[name] {
				continue
			}
			selectedUser, e := p.API.GetUserByUsername(name)
			if e != nil {
				return nil, fmt.Errorf("用户 @%s 不存在", name)
			}
			if selectedUser.DeleteAt != 0 || selectedUser.IsBot {
				return nil, fmt.Errorf("用户 @%s 不存在", name)
			}
			if _, e := p.API.GetChannelMember(req.ChannelID, selectedUser.Id); e != nil {
				return nil, fmt.Errorf("用户 @%s 不是该频道成员", name)
			}
			uniq[name] = true
			usernames = append(usernames, name)
			targetUsers = append(targetUsers, ReminderUser{ID: selectedUser.Id, Username: selectedUser.Username})
		}
	} else {
		targetUsers, err = p.getChannelReminderUsers(req.ChannelID)
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(req.Rules, func(i, j int) bool { return req.Rules[i].FireAt < req.Rules[j].FireAt })
	now := time.Now().UnixMilli()
	return &Reminder{ID: model.NewId(), CreatorID: userID, CreatorUsername: user.Username, ChannelID: req.ChannelID, Title: req.Title, Content: req.Content, DueDate: req.DueDate, Timezone: req.Timezone, Audience: req.Audience, Usernames: usernames, TargetUsers: targetUsers, Rules: req.Rules, CreatedAt: now, ActionToken: model.NewId()}, nil
}

func (p *Plugin) getChannelReminderUsers(channelID string) ([]ReminderUser, error) {
	const pageSize = 200
	result := make([]ReminderUser, 0)
	for page := 0; ; page++ {
		users, appErr := p.API.GetUsersInChannel(channelID, "username", page, pageSize)
		if appErr != nil {
			return nil, fmt.Errorf("无法读取频道成员")
		}
		for _, user := range users {
			if user.DeleteAt == 0 && !user.IsBot {
				result = append(result, ReminderUser{ID: user.Id, Username: user.Username})
			}
		}
		if len(users) < pageSize {
			break
		}
	}
	return result, nil
}

func reminderKey(id string) string { return "reminder_" + id }

func (p *Plugin) saveReminder(rem *Reminder) error {
	p.storeLock.Lock()
	defer p.storeLock.Unlock()
	ids, err := p.loadIndex()
	if err != nil {
		return err
	}
	reminderData, err := json.Marshal(rem)
	if err != nil {
		return err
	}
	if err := p.API.KVSet(reminderKey(rem.ID), reminderData); err != nil {
		return err
	}
	ids = append(ids, rem.ID)
	indexData, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	if appErr := p.API.KVSet(indexKey, indexData); appErr != nil {
		return appErr
	}
	return nil
}

func (p *Plugin) updateReminder(rem *Reminder) error {
	data, err := json.Marshal(rem)
	if err != nil {
		return err
	}
	if appErr := p.API.KVSet(reminderKey(rem.ID), data); appErr != nil {
		return appErr
	}
	return nil
}

func (p *Plugin) getReminder(reminderID string) (*Reminder, error) {
	p.storeLock.Lock()
	defer p.storeLock.Unlock()
	data, appErr := p.API.KVGet(reminderKey(reminderID))
	if appErr != nil || len(data) == 0 {
		return nil, fmt.Errorf("reminder not found")
	}
	var reminder Reminder
	if err := json.Unmarshal(data, &reminder); err != nil {
		return nil, err
	}
	return &reminder, nil
}

func (p *Plugin) replaceReminder(reminder *Reminder) error {
	p.storeLock.Lock()
	defer p.storeLock.Unlock()
	return p.updateReminder(reminder)
}

func (p *Plugin) setUserStatus(reminderID, token, userID, channelID, status string) (*Reminder, bool, error) {
	p.storeLock.Lock()
	defer p.storeLock.Unlock()
	data, appErr := p.API.KVGet(reminderKey(reminderID))
	if appErr != nil || len(data) == 0 {
		return nil, false, fmt.Errorf("リマインダーが見つかりません。")
	}
	var reminder Reminder
	if err := json.Unmarshal(data, &reminder); err != nil {
		return nil, false, fmt.Errorf("リマインダーを読み取れませんでした。")
	}
	if reminder.ActionToken == "" || reminder.ActionToken != token || reminder.ChannelID != channelID {
		return nil, false, fmt.Errorf("確認リクエストが無効です。")
	}
	if reminder.DeletedAt != 0 {
		return nil, false, fmt.Errorf("このリマインダーは削除されています。")
	}
	var target *ReminderUser
	for i := range reminder.TargetUsers {
		if reminder.TargetUsers[i].ID == userID {
			target = &reminder.TargetUsers[i]
			break
		}
	}
	if target == nil {
		return nil, false, fmt.Errorf("このリマインダーの対象ユーザーではありません。")
	}
	for _, completion := range reminder.Completions {
		if completion.UserID == userID {
			return &reminder, true, nil
		}
	}
	if status == "acknowledged" {
		for _, acknowledgement := range reminder.Acknowledgements {
			if acknowledgement.UserID == userID {
				return &reminder, true, nil
			}
		}
		reminder.Acknowledgements = append(reminder.Acknowledgements, Acknowledgement{UserID: userID, Username: target.Username, AcknowledgedAt: time.Now().UnixMilli()})
	} else if status == "completed" {
		acknowledgements := reminder.Acknowledgements[:0]
		for _, acknowledgement := range reminder.Acknowledgements {
			if acknowledgement.UserID != userID {
				acknowledgements = append(acknowledgements, acknowledgement)
			}
		}
		reminder.Acknowledgements = acknowledgements
		reminder.Completions = append(reminder.Completions, Completion{UserID: userID, Username: target.Username, CompletedAt: time.Now().UnixMilli()})
	} else {
		return nil, false, fmt.Errorf("確認状態が無効です。")
	}
	if err := p.updateReminder(&reminder); err != nil {
		return nil, false, fmt.Errorf("対応状況を保存できませんでした。")
	}
	return &reminder, false, nil
}

func (p *Plugin) loadIndex() ([]string, error) {
	data, appErr := p.API.KVGet(indexKey)
	if appErr != nil {
		return nil, appErr
	}
	if len(data) == 0 {
		return []string{}, nil
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func (p *Plugin) listReminders(channelID string) ([]Reminder, error) {
	p.storeLock.Lock()
	defer p.storeLock.Unlock()
	ids, err := p.loadIndex()
	if err != nil {
		return nil, err
	}
	reminders := make([]Reminder, 0)
	for _, id := range ids {
		data, appErr := p.API.KVGet(reminderKey(id))
		if appErr != nil {
			return nil, appErr
		}
		if len(data) == 0 {
			continue
		}
		var reminder Reminder
		if err := json.Unmarshal(data, &reminder); err != nil {
			return nil, err
		}
		if reminder.ChannelID == channelID {
			reminders = append(reminders, reminder)
		}
	}
	sort.Slice(reminders, func(i, j int) bool { return reminders[i].CreatedAt > reminders[j].CreatedAt })
	return reminders, nil
}

func (p *Plugin) runScheduler() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.processDue()
		case <-p.stop:
			return
		}
	}
}

func (p *Plugin) processDue() {
	p.storeLock.Lock()
	defer p.storeLock.Unlock()
	ids, err := p.loadIndex()
	if err != nil {
		p.API.LogError("load reminder index", "error", err.Error())
		return
	}
	now := time.Now().UnixMilli()
	for _, id := range ids {
		var rem Reminder
		data, appErr := p.API.KVGet(reminderKey(id))
		if appErr != nil || len(data) == 0 {
			continue
		}
		if err := json.Unmarshal(data, &rem); err != nil {
			p.API.LogError("decode reminder", "id", id, "error", err.Error())
			continue
		}
		if rem.DeletedAt != 0 {
			continue
		}
		changed := false
		for i := range rem.Rules {
			if rem.Rules[i].FiredAt != 0 || rem.Rules[i].FireAt > now {
				continue
			}
			if !allTargetsCompleted(&rem) {
				post, err := p.createReminderPost(&rem, false)
				if err != nil {
					p.API.LogError("deliver reminder", "id", id, "error", err.Error())
					continue
				}
				rem.Posts = append(reminderPostRecords(&rem), ReminderPost{ID: post.Id, Announcement: false})
			}
			rem.Rules[i].FiredAt = time.Now().UnixMilli()
			changed = true
		}
		if changed {
			updated, err := json.Marshal(&rem)
			if err != nil {
				p.API.LogError("encode reminder", "id", id, "error", err.Error())
				continue
			}
			if err := p.API.KVSet(reminderKey(id), updated); err != nil {
				p.API.LogError("mark reminder fired", "id", id, "error", err.Error())
			}
		}
	}
}

func allTargetsCompleted(reminder *Reminder) bool {
	return len(reminder.TargetUsers) > 0 && len(reminder.Completions) >= len(reminder.TargetUsers)
}

func pendingUsers(reminder *Reminder) []ReminderUser {
	done := make(map[string]bool, len(reminder.Completions))
	for _, completion := range reminder.Completions {
		done[completion.UserID] = true
	}
	pending := make([]ReminderUser, 0, len(reminder.TargetUsers))
	for _, user := range reminder.TargetUsers {
		if !done[user.ID] {
			pending = append(pending, user)
		}
	}
	return pending
}

func unacknowledgedUsers(reminder *Reminder) []ReminderUser {
	known := make(map[string]bool, len(reminder.Acknowledgements)+len(reminder.Completions))
	for _, acknowledgement := range reminder.Acknowledgements {
		known[acknowledgement.UserID] = true
	}
	for _, completion := range reminder.Completions {
		known[completion.UserID] = true
	}
	users := make([]ReminderUser, 0, len(reminder.TargetUsers))
	for _, user := range reminder.TargetUsers {
		if !known[user.ID] {
			users = append(users, user)
		}
	}
	return users
}

func userMentions(users []ReminderUser) string {
	parts := make([]string, len(users))
	for i, user := range users {
		parts[i] = "@" + user.Username
	}
	return strings.Join(parts, " ")
}

func (p *Plugin) createReminderPost(rem *Reminder, announcement bool) (*model.Post, error) {
	if p.botUserID == "" {
		return nil, fmt.Errorf("reminder bot is not initialized")
	}
	mention := "@" + rem.Audience
	if announcement {
		if rem.Audience == "users" {
			mention = userMentions(rem.TargetUsers)
		}
	} else if len(rem.TargetUsers) > 0 {
		mention = userMentions(pendingUsers(rem))
	}
	post := &model.Post{UserId: p.botUserID, ChannelId: rem.ChannelID, Type: reminderPostType, Message: reminderFallbackMessage(rem, announcement, mention)}
	setReminderPostProps(post, rem, announcement)
	created, appErr := p.API.CreatePost(post)
	if appErr != nil {
		return nil, fmt.Errorf("create post: %s", appErr.Error())
	}
	return created, nil
}

func reminderTitle(reminder *Reminder) string {
	if strings.TrimSpace(reminder.Title) == "" {
		return "Deadline reminder"
	}
	return reminder.Title
}

func reminderFallbackMessage(reminder *Reminder, announcement bool, mention string) string {
	unhandled := unacknowledgedUsers(reminder)
	message := fmt.Sprintf("🔔 %s **%s**\n\n%s\n\nDue date: **%s** (%s) · Created by: @%s", mention, reminderTitle(reminder), fencedContent(reminder.Content), reminder.DueDate, reminder.Timezone, reminder.CreatorUsername)
	if announcement {
		message += "\n\nFollow-up reminder times:\n" + reminderSchedule(reminder)
	}
	message += fmt.Sprintf("\n\n**Not handled (%d):** %s\n**Acknowledged (%d):** %s\n**Handled (%d):** %s", len(unhandled), fallbackUserMentions(unhandled), len(reminder.Acknowledgements), fallbackAcknowledgementMentions(reminder.Acknowledgements), len(reminder.Completions), fallbackCompletionMentions(reminder.Completions))
	if reminder.DeletedAt != 0 {
		message = strikeMessage(message)
	}
	return message
}

func setReminderPostProps(post *model.Post, reminder *Reminder, announcement bool) {
	post.AddProp("reminder_id", reminder.ID)
	post.AddProp("reminder_announcement", fmt.Sprintf("%t", announcement))
	post.AddProp("reminder_revision", time.Now().UnixMilli())
}

func fencedContent(content string) string {
	fence := "```"
	for strings.Contains(content, fence) {
		fence += "`"
	}
	return fence + "\n" + content + "\n" + fence
}

func strikeMessage(message string) string {
	lines := strings.Split(message, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "~~" + line + "~~"
		}
	}
	return strings.Join(lines, "\n")
}

func reminderSchedule(reminder *Reminder) string {
	location, err := time.LoadLocation(reminder.Timezone)
	if err != nil {
		location = time.UTC
	}
	lines := make([]string, len(reminder.Rules))
	for i, rule := range reminder.Rules {
		lines[i] = "- " + time.UnixMilli(rule.FireAt).In(location).Format("2006-01-02 15:04")
	}
	return strings.Join(lines, "\n")
}

func (p *Plugin) setReminderAttachment(post *model.Post, reminder *Reminder) {
	pending := pendingUsers(reminder)
	unacknowledged := unacknowledgedUsers(reminder)
	status := fmt.Sprintf("**未対応 (%d):** %s\n**了解 (%d):** %s\n**対応済み (%d):** %s", len(unacknowledged), statusUserMentions(unacknowledged), len(reminder.Acknowledgements), statusAcknowledgementMentions(reminder.Acknowledgements), len(reminder.Completions), statusCompletionMentions(reminder.Completions))
	actions := make([]*model.PostAction, 0, 2)
	if len(pending) > 0 && reminder.ActionToken != "" && reminder.DeletedAt == 0 {
		actions = append(actions, &model.PostAction{
			Id:    "acknowledge",
			Type:  model.PostActionTypeButton,
			Name:  "了解",
			Style: "primary",
			Integration: &model.PostActionIntegration{
				URL:     "/plugins/" + pluginID + "/api/v1/actions/acknowledge",
				Context: map[string]any{"reminder_id": reminder.ID, "token": reminder.ActionToken},
			},
		})
		actions = append(actions, &model.PostAction{
			Id:    "complete",
			Type:  model.PostActionTypeButton,
			Name:  "対応済み",
			Style: "success",
			Integration: &model.PostActionIntegration{
				URL:     "/plugins/" + pluginID + "/api/v1/actions/complete",
				Context: map[string]any{"reminder_id": reminder.ID, "token": reminder.ActionToken},
			},
		})
	}
	post.DelProp(model.PostPropsAttachments)
	model.ParseSlackAttachment(post, []*model.SlackAttachment{{Color: "#E7A23B", Text: status, Actions: actions}})
}

func statusUserMentions(users []ReminderUser) string {
	if len(users) == 0 {
		return "なし"
	}
	return userMentions(users)
}

func statusCompletionMentions(completions []Completion) string {
	if len(completions) == 0 {
		return "なし"
	}
	parts := make([]string, len(completions))
	for i, completion := range completions {
		parts[i] = "@" + completion.Username
	}
	return strings.Join(parts, " ")
}

func statusAcknowledgementMentions(acknowledgements []Acknowledgement) string {
	if len(acknowledgements) == 0 {
		return "なし"
	}
	parts := make([]string, len(acknowledgements))
	for i, acknowledgement := range acknowledgements {
		parts[i] = "@" + acknowledgement.Username
	}
	return strings.Join(parts, " ")
}

func reminderPostRecords(reminder *Reminder) []ReminderPost {
	if len(reminder.Posts) > 0 {
		return reminder.Posts
	}
	if reminder.AnnouncementPostID != "" {
		return []ReminderPost{{ID: reminder.AnnouncementPostID, Announcement: true}}
	}
	return nil
}

func (p *Plugin) updateAllReminderPosts(reminder *Reminder) {
	for _, post := range reminderPostRecords(reminder) {
		p.updateReminderPost(post, reminder)
	}
}

func (p *Plugin) updateReminderPost(reminderPost ReminderPost, reminder *Reminder) {
	post, appErr := p.API.GetPost(reminderPost.ID)
	if appErr != nil || post.UserId != p.botUserID || post.ChannelId != reminder.ChannelID {
		return
	}
	mention := "@" + reminder.Audience
	if reminderPost.Announcement {
		if reminder.Audience == "users" {
			mention = userMentions(reminder.TargetUsers)
		}
	} else if len(reminder.TargetUsers) > 0 {
		mention = userMentions(pendingUsers(reminder))
	}
	post.Type = reminderPostType
	post.Message = reminderFallbackMessage(reminder, reminderPost.Announcement, mention)
	post.DelProp(model.PostPropsAttachments)
	setReminderPostProps(post, reminder, reminderPost.Announcement)
	if _, appErr := p.API.UpdatePost(post); appErr != nil {
		p.API.LogError("update reminder post", "post_id", reminderPost.ID, "error", appErr.Error())
	}
}

func (p *Plugin) migrateReminderPosts() {
	p.storeLock.Lock()
	ids, err := p.loadIndex()
	p.storeLock.Unlock()
	if err != nil {
		p.API.LogError("load reminder index for post migration", "error", err.Error())
		return
	}
	for _, id := range ids {
		reminder, err := p.getReminder(id)
		if err != nil {
			continue
		}
		for _, record := range reminderPostRecords(reminder) {
			post, appErr := p.API.GetPost(record.ID)
			if appErr == nil && post != nil && post.Type == reminderPostType {
				continue
			}
			p.updateReminderPost(record, reminder)
		}
	}
}

func fallbackUserMentions(users []ReminderUser) string {
	if len(users) == 0 {
		return "None"
	}
	return userMentions(users)
}

func fallbackCompletionMentions(completions []Completion) string {
	if len(completions) == 0 {
		return "None"
	}
	parts := make([]string, len(completions))
	for i, completion := range completions {
		parts[i] = "@" + completion.Username
	}
	return strings.Join(parts, " ")
}

func fallbackAcknowledgementMentions(acknowledgements []Acknowledgement) string {
	if len(acknowledgements) == 0 {
		return "None"
	}
	parts := make([]string, len(acknowledgements))
	for i, acknowledgement := range acknowledgements {
		parts[i] = "@" + acknowledgement.Username
	}
	return strings.Join(parts, " ")
}
