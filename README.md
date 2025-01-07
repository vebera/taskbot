# TaskBot - Discord Time Tracking Bot

TaskBot is a Discord bot designed to help teams track time spent on tasks, manage work items, and generate reports. It provides an easy-to-use interface for time tracking and task management directly within Discord.

## Features

- **Task Management**
  - Create personal and global tasks
  - Track task status (Open/Completed)
  - Automatic task suggestions with autocomplete

- **Time Tracking**
  - Check in/out of tasks
  - Declare time spent on tasks
  - Real-time status updates
  - Timezone support for accurate time tracking

- **Reporting**
  - Generate time reports for various periods (Today, Week, Month)
  - Export reports in Text or CSV format (CSV for admins)
  - Filter reports by username
  - View current task status for all users

## Commands

### Basic Commands
- `/checkin` - Start working on a task
  - `existing` - Check in to an existing task
  - `new` - Create and check in to a new task
- `/checkout` - Stop working on the current task
- `/status` - Show current task status for all users
- `/declare` - Declare time spent on a task

### Task Management
- `/task` - Update task status (Open/Completed)
- `/globaltask` - Create a global task visible to everyone (admin only)

### Time and Reporting
- `/timezone` - Set your timezone (e.g., America/New_York, Europe/London)
- `/report` - Generate task history reports
  - Time periods: Today, This Week, This Month, Last Month, up to 6 Months Ago
  - Output formats: Text, CSV (admin only)
  - Optional username filter

## Setup

1. Create a Discord application and bot token
2. Set up the required environment variables (see `.env.example`)
3. Configure the database (PostgreSQL)
4. Run the migrations
5. Start the bot using Docker or directly with Go

## Environment Variables

Copy `.env.example` to `.env` and configure the following:
- `DISCORD_TOKEN` - Your Discord bot token
- `DATABASE_URL` - PostgreSQL connection string
- Other configuration options as needed

## Development

The project uses:
- Go for the backend
- PostgreSQL for data storage
- Discord API for bot interactions

## License

MIT License

Copyright (c) 2024 TaskBot

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE. 