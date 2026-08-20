package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	pluginID = "com.cw.remind"
	indexKey = "reminder_index"
)

type Plugin struct {
	plugin.MattermostPlugin
	stop      chan struct{}
	storeLock sync.Mutex
}

func (p *Plugin) OnActivate() error {
	if err := p.API.RegisterCommand(&model.Command{Trigger: "remind", AutoComplete: true, AutoCompleteDesc: "期限リマインダーを設定 / 设置期限提醒", DisplayName: "CW Remind"}); err != nil {
		return fmt.Errorf("register /remind: %w", err)
	}
	p.stop = make(chan struct{})
	go p.runScheduler()
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
	if r.URL.Path != "/api/v1/reminders" || r.Method != http.MethodPost {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		return
	}
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
	if err := p.saveReminder(reminder); err != nil {
		p.API.LogError("save reminder", "error", err.Error())
		writeError(w, 500, "could not save reminder")
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(reminder)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (p *Plugin) validateAndBuild(userID string, req createReminderRequest) (*Reminder, error) {
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" || len([]rune(req.Content)) > 4000 {
		return nil, fmt.Errorf("提醒内容必须为 1–4000 字符")
	}
	if req.ChannelID == "" {
		return nil, fmt.Errorf("频道不能为空")
	}
	if _, appErr := p.API.GetChannelMember(req.ChannelID, userID); appErr != nil {
		return nil, fmt.Errorf("你不是该频道成员")
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
			if _, e := p.API.GetUserByUsername(name); e != nil {
				return nil, fmt.Errorf("用户 @%s 不存在", name)
			}
			uniq[name] = true
			usernames = append(usernames, name)
		}
	}
	sort.Slice(req.Rules, func(i, j int) bool { return req.Rules[i].FireAt < req.Rules[j].FireAt })
	now := time.Now().UnixMilli()
	return &Reminder{ID: model.NewId(), CreatorID: userID, CreatorUsername: user.Username, ChannelID: req.ChannelID, Content: req.Content, DueDate: req.DueDate, Timezone: req.Timezone, Audience: req.Audience, Usernames: usernames, Rules: req.Rules, CreatedAt: now}, nil
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
	return p.API.KVSet(indexKey, indexData)
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

func (p *Plugin) runScheduler() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	p.processDue()
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
		changed := false
		for i := range rem.Rules {
			if rem.Rules[i].FiredAt != 0 || rem.Rules[i].FireAt > now {
				continue
			}
			if err := p.deliver(&rem); err != nil {
				p.API.LogError("deliver reminder", "id", id, "error", err.Error())
				continue
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

func (p *Plugin) deliver(rem *Reminder) error {
	mention := "@" + rem.Audience
	if rem.Audience == "users" {
		parts := make([]string, len(rem.Usernames))
		for i, u := range rem.Usernames {
			parts[i] = "@" + u
		}
		mention = strings.Join(parts, " ")
	}
	message := fmt.Sprintf("🔔 %s **期限提醒**\n\n%s\n\n期限: **%s** (%s) · 建立者: @%s", mention, rem.Content, rem.DueDate, rem.Timezone, rem.CreatorUsername)
	_, appErr := p.API.CreatePost(&model.Post{ChannelId: rem.ChannelID, Message: message})
	if appErr != nil {
		return fmt.Errorf("create post: %s", appErr.Error())
	}
	return nil
}
