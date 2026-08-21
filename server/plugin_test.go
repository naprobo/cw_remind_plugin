package main

import (
	"encoding/json"
	"testing"

	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSaveReminderReturnsNilAfterSuccessfulKVWrites(t *testing.T) {
	api := &plugintest.API{}
	reminder := &Reminder{ID: "reminder-id"}

	api.On("KVGet", indexKey).Return([]byte(nil), nil).Once()
	api.On("KVSet", reminderKey(reminder.ID), mock.Anything).Return(nil).Once()
	api.On("KVSet", indexKey, mock.Anything).Return(nil).Once()

	p := &Plugin{}
	p.SetAPI(api)

	require.NoError(t, p.saveReminder(reminder))
	api.AssertExpectations(t)
}

func TestListRemindersShowsAllChannelCreatorsAndSortsNewestFirst(t *testing.T) {
	api := &plugintest.API{}
	reminders := map[string]Reminder{
		"older":         {ID: "older", CreatorID: "user-1", ChannelID: "channel-1", CreatedAt: 100},
		"newer":         {ID: "newer", CreatorID: "user-1", ChannelID: "channel-1", CreatedAt: 300},
		"other-user":    {ID: "other-user", CreatorID: "user-2", ChannelID: "channel-1", CreatedAt: 400},
		"other-channel": {ID: "other-channel", CreatorID: "user-1", ChannelID: "channel-2", CreatedAt: 500},
	}
	ids, err := json.Marshal([]string{"older", "other-channel", "newer", "other-user"})
	require.NoError(t, err)
	api.On("KVGet", indexKey).Return(ids, nil).Once()
	for _, id := range []string{"older", "other-channel", "newer", "other-user"} {
		data, marshalErr := json.Marshal(reminders[id])
		require.NoError(t, marshalErr)
		api.On("KVGet", reminderKey(id)).Return(data, nil).Once()
	}

	p := &Plugin{}
	p.SetAPI(api)

	got, err := p.listReminders("channel-1")
	require.NoError(t, err)
	require.Equal(t, []string{"other-user", "newer", "older"}, []string{got[0].ID, got[1].ID, got[2].ID})
	api.AssertExpectations(t)
}

func TestPublicReminderHidesActionToken(t *testing.T) {
	reminder := &Reminder{ID: "reminder-id", ActionToken: "private-token"}

	public := publicReminder(reminder)

	require.Empty(t, public.ActionToken)
	require.Equal(t, "private-token", reminder.ActionToken)
}

func TestCompleteReminderRecordsTargetUser(t *testing.T) {
	api := &plugintest.API{}
	reminder := Reminder{
		ID:          "reminder-id",
		ChannelID:   "channel-id",
		ActionToken: "private-token",
		TargetUsers: []ReminderUser{{ID: "user-id", Username: "alice"}},
	}
	data, err := json.Marshal(reminder)
	require.NoError(t, err)
	api.On("KVGet", reminderKey(reminder.ID)).Return(data, nil).Once()
	api.On("KVSet", reminderKey(reminder.ID), mock.Anything).Return(nil).Once()

	p := &Plugin{}
	p.SetAPI(api)
	completed, alreadyCompleted, err := p.setUserStatus(reminder.ID, reminder.ActionToken, "user-id", reminder.ChannelID, "completed")

	require.NoError(t, err)
	require.False(t, alreadyCompleted)
	require.Len(t, completed.Completions, 1)
	require.Equal(t, "alice", completed.Completions[0].Username)
	require.True(t, allTargetsCompleted(completed))
	api.AssertExpectations(t)
}

func TestCompleteReminderRejectsNonTargetUser(t *testing.T) {
	api := &plugintest.API{}
	reminder := Reminder{
		ID:          "reminder-id",
		ChannelID:   "channel-id",
		ActionToken: "private-token",
		TargetUsers: []ReminderUser{{ID: "user-id", Username: "alice"}},
	}
	data, err := json.Marshal(reminder)
	require.NoError(t, err)
	api.On("KVGet", reminderKey(reminder.ID)).Return(data, nil).Once()

	p := &Plugin{}
	p.SetAPI(api)
	_, _, err = p.setUserStatus(reminder.ID, reminder.ActionToken, "other-user", reminder.ChannelID, "completed")

	require.EqualError(t, err, "このリマインダーの対象ユーザーではありません。")
	api.AssertExpectations(t)
}

func TestAcknowledgementRemainsPending(t *testing.T) {
	api := &plugintest.API{}
	reminder := Reminder{
		ID:          "reminder-id",
		ChannelID:   "channel-id",
		ActionToken: "private-token",
		TargetUsers: []ReminderUser{{ID: "user-id", Username: "alice"}},
	}
	data, err := json.Marshal(reminder)
	require.NoError(t, err)
	api.On("KVGet", reminderKey(reminder.ID)).Return(data, nil).Once()
	api.On("KVSet", reminderKey(reminder.ID), mock.Anything).Return(nil).Once()

	p := &Plugin{}
	p.SetAPI(api)
	acknowledged, unchanged, err := p.setUserStatus(reminder.ID, reminder.ActionToken, "user-id", reminder.ChannelID, "acknowledged")

	require.NoError(t, err)
	require.False(t, unchanged)
	require.Len(t, acknowledged.Acknowledgements, 1)
	require.Equal(t, []ReminderUser{{ID: "user-id", Username: "alice"}}, pendingUsers(acknowledged))
	require.False(t, allTargetsCompleted(acknowledged))
	api.AssertExpectations(t)
}

func TestFencedContentUsesLongerFenceForEmbeddedBackticks(t *testing.T) {
	require.Equal(t, "```\nplain\n```", fencedContent("plain"))
	require.Equal(t, "````\ninside ``` fence\n````", fencedContent("inside ``` fence"))
}
