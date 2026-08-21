package main

type Rule struct {
	DaysBefore int    `json:"days_before"`
	Time       string `json:"time"`
	FireAt     int64  `json:"fire_at"`
	FiredAt    int64  `json:"fired_at,omitempty"`
}

type Reminder struct {
	ID                 string            `json:"id"`
	CreatorID          string            `json:"creator_id"`
	CreatorUsername    string            `json:"creator_username"`
	ChannelID          string            `json:"channel_id"`
	Title              string            `json:"title"`
	Content            string            `json:"content"`
	DueDate            string            `json:"due_date"`
	Timezone           string            `json:"timezone"`
	Audience           string            `json:"audience"`
	Usernames          []string          `json:"usernames,omitempty"`
	TargetUsers        []ReminderUser    `json:"target_users,omitempty"`
	Acknowledgements   []Acknowledgement `json:"acknowledgements,omitempty"`
	Completions        []Completion      `json:"completions,omitempty"`
	Rules              []Rule            `json:"rules"`
	CreatedAt          int64             `json:"created_at"`
	AnnouncementPostID string            `json:"announcement_post_id,omitempty"`
	Posts              []ReminderPost    `json:"posts,omitempty"`
	ActionToken        string            `json:"action_token,omitempty"`
	UpdatedAt          int64             `json:"updated_at,omitempty"`
	DeletedAt          int64             `json:"deleted_at,omitempty"`
	DeletedBy          string            `json:"deleted_by,omitempty"`
}

type ReminderUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type Completion struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	CompletedAt int64  `json:"completed_at"`
}

type Acknowledgement struct {
	UserID         string `json:"user_id"`
	Username       string `json:"username"`
	AcknowledgedAt int64  `json:"acknowledged_at"`
}

type ReminderPost struct {
	ID           string `json:"id"`
	Announcement bool   `json:"announcement"`
}

type createReminderRequest struct {
	ChannelID string   `json:"channel_id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	DueDate   string   `json:"due_date"`
	Timezone  string   `json:"timezone"`
	Audience  string   `json:"audience"`
	Usernames []string `json:"usernames"`
	Rules     []Rule   `json:"rules"`
}

type channelUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}
