package main

type Rule struct {
	DaysBefore int    `json:"days_before"`
	Time       string `json:"time"`
	FireAt     int64  `json:"fire_at"`
	FiredAt    int64  `json:"fired_at,omitempty"`
}

type Reminder struct {
	ID              string   `json:"id"`
	CreatorID       string   `json:"creator_id"`
	CreatorUsername string   `json:"creator_username"`
	ChannelID       string   `json:"channel_id"`
	Content         string   `json:"content"`
	DueDate         string   `json:"due_date"`
	Timezone        string   `json:"timezone"`
	Audience        string   `json:"audience"`
	Usernames       []string `json:"usernames,omitempty"`
	Rules           []Rule   `json:"rules"`
	CreatedAt       int64    `json:"created_at"`
}

type createReminderRequest struct {
	ChannelID string   `json:"channel_id"`
	Content   string   `json:"content"`
	DueDate   string   `json:"due_date"`
	Timezone  string   `json:"timezone"`
	Audience  string   `json:"audience"`
	Usernames []string `json:"usernames"`
	Rules     []Rule   `json:"rules"`
}
