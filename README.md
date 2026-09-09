# CW Remind for Mattermost

CW Remind adds multi-stage deadline reminders to Mattermost. Open the reminder dialog with `/remind` or the megaphone button in the channel header, then schedule one or more notifications before a due date.

## Features

- Publishes a reminder announcement immediately through the `DueWatch` bot.
- Supports multiple notification times for a single due date.
- Notifies everyone in the channel or selected channel members.
- Lets recipients acknowledge a reminder or mark it as completed directly from the post.
- Excludes completed recipients from later notifications.
- Provides a channel history view with newest reminders first.
- Lets the creator edit or soft-delete a reminder while preserving its history.
- Updates existing posts after edits and strikes through posts after deletion.
- Uses English by default and follows the Mattermost user locale for Japanese and Simplified Chinese.
- Works on desktop and mobile layouts.

## Requirements

- Mattermost 9.5 or later. Mattermost 10.12 is the primary tested version.
- System administrator access to install and enable plugins.
- Plugin uploads enabled on the Mattermost server when installing through the System Console.

## Installation

1. Download `com.cw.remind-0.1.1.tar.gz` from the project releases.
2. In Mattermost, open **System Console > Plugins > Plugin Management**.
3. Upload the archive and enable **CW Remind**.
4. Open a channel and confirm that the megaphone button appears in the channel header.

Administrators using local mode can install the archive with `mmctl`:

```bash
mmctl plugin add com.cw.remind-0.1.1.tar.gz --local
mmctl plugin enable com.cw.remind --local
```

When replacing a build that uses the same version number, disable and remove the installed plugin first, install the new archive, and refresh the Mattermost browser tab.

## Usage

1. Enter `/remind` or click the megaphone button in the current channel.
2. Enter a title, reminder text, due date, and one or more notification times.
3. Choose **Everyone** or **Specific users**. Specific users must be current members of the channel.
4. Create the reminder. DueWatch immediately posts the title, content, due date, and notification schedule.
5. Recipients can select **Acknowledged** or **Completed** from the reminder post.
6. Open the **History** tab to review, edit, or delete reminders in the current channel.

Only the reminder creator can edit or delete it. Deleted reminders remain visible in history but no longer send notifications.

## Uninstallation

Disable and remove `com.cw.remind` from **System Console > Plugins > Plugin Management**, or run:

```bash
mmctl plugin disable com.cw.remind --local
mmctl plugin delete com.cw.remind --local
```
