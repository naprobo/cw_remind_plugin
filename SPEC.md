# CW Remind Product Specification

## 1. Purpose

CW Remind is a Mattermost deadline-reminder plugin. A user opens the reminder dialog with `/remind` or the channel-header button and creates one deadline event with one or more notification times.

The plugin ID is `com.cw.remind`. The bot username is `duewatch`, and its display name is `DueWatch`.

## 2. Reminder Creation

- Title is required and contains 1 to 200 characters.
- Reminder content is required and contains 1 to 4,000 characters.
- Due date is required and uses a calendar date.
- Notification audience is either everyone or one or more selected Mattermost users.
- Selected users are searchable current members of the channel. Deleted users and bots are excluded.
- At least one notification rule is required. Each rule contains a number of days before the due date and a local time.
- Days before may be zero to represent the due date itself.
- A reminder cannot contain duplicate day-and-time rules.
- The creator's browser submits an IANA timezone, which is stored with the reminder and used to calculate UTC trigger timestamps.
- All notification timestamps must be in the future when the reminder is saved.

## 3. Delivery Behavior

- Activating the plugin creates or reuses DueWatch and applies the bundled megaphone profile image.
- Creating a reminder immediately publishes an announcement in the source channel.
- The announcement contains the title, safely fenced reminder content, due date, creator, audience, and all pending notification times.
- An everyone reminder includes `@all`. A selected-user reminder mentions each selected username.
- The server stores an audience snapshot at creation. Everyone includes all active non-bot channel members; selected-user reminders include only the selected members.
- Announcement and notification posts provide **Acknowledged** and **Completed** actions.
- Acknowledged means the recipient knows about the event but still requires follow-up.
- Completed means the recipient no longer receives later notifications for that reminder.
- Later posts mention only recipients who are not completed and show pending, acknowledged, and completed groups.
- When every target recipient is completed, later rules are marked as processed without publishing another post.
- The scheduler scans every 30 seconds and records successful deliveries to prevent normal polling duplicates.

## 4. History and Management

- The dialog contains Create and History tabs.
- History lists every reminder in the current channel, newest first.
- Each entry shows its title, content, creator, audience, schedule status, and recipient status.
- Every channel member may view channel reminder history.
- Only the creator may edit or delete a reminder.
- Editing recalculates future notification timestamps and updates all posts associated with the reminder.
- Recipient acknowledgement and completion state is retained only for users who remain in the updated audience.
- Deletion is soft deletion. It stops scheduling, keeps the history record, removes active actions, and strikes through existing posts.

## 5. Permissions and Validation

- Unauthenticated GUI API requests return HTTP 401.
- A user may create a reminder only in a channel where that user is currently a member.
- Every selected recipient must be an active non-bot member of the same channel.
- Only target recipients may use acknowledgement and completion actions.
- Interactive actions validate the Mattermost user, channel, post, reminder ID, and private action token.
- Invalid dates, times, timezones, audiences, users, duplicate rules, and expired schedules are rejected without storage.
- Public API responses never expose the private action token.

## 6. Localization

- English is the default interface language.
- Japanese and Simplified Chinese are supported.
- The interface follows the current user's Mattermost locale and falls back to English for unsupported locales.
- Server errors are mapped to localized user-facing messages where a known mapping exists.

## 7. Data Model

```text
Reminder
  id, creator_id, creator_username, channel_id
  title, content, due_date, timezone
  audience: all | users
  usernames[]
  target_users[]: {id, username}
  acknowledgements[]: {user_id, username, acknowledged_at}
  completions[]: {user_id, username, completed_at}
  rules[]: {days_before, time, fire_at, fired_at?}
  posts[]: {id, announcement}
  announcement_post_id, action_token
  created_at, updated_at, deleted_at, deleted_by
```

Legacy records may contain the `channel` audience. It behaves as a channel-wide audience, but the current UI exposes only everyone and selected users.

## 8. Acceptance Criteria

- `/remind` and the channel-header button open the same dialog for the current channel.
- A reminder may contain multiple valid notification rules, including a rule on the due date.
- Creating a reminder immediately publishes its title, content, deadline, and notification schedule.
- Selected recipients must come from the current channel.
- A target recipient can move from pending to acknowledged and then to completed.
- Later notifications mention only recipients who are not completed.
- No later post is created after every target recipient is completed.
- Editing updates existing posts and preserves applicable recipient state.
- Soft deletion preserves history, stops future delivery, and strikes through existing posts.
- Reminders survive a Mattermost restart.
- Under normal single-node operation, each rule is delivered once.
- English, Japanese, and Simplified Chinese interfaces follow the Mattermost locale.
