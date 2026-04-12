# Jacksonian Gaming Council

Me and three friends run 12-game board game seasons. I would like to create an application we can use to see information about current and previous seasons.

## Data display
- Current leaderboard
- Game history
- Graph of each person's points
- Click on a season to view a diferent season
- Click on a person to view their game and season history
- Click on a game to view its play history

## Data entry
- I'll need to add info for previous seasons
- Should have a feature to add the results of new games that are played
- We should require authentication for this so that randoms can't do it

## Technology
- HTMX + vanilla JS
- Go stdlib backend
- SQLLite
- Deployed to my VPS so we should use docker

---

## Feature Plan

### F1 — Foundation
- SQLite schema: players, seasons, games, game_results tables
- Go stdlib HTTP server skeleton with routing
- Docker setup (Dockerfile + compose)
- Static file serving for HTMX/JS assets

### F2 — Authentication
- Simple session-based auth (no OAuth needed for 4 people)
- Login page, protected routes for data entry
- Hardcoded users or a small admin table

### F3 — Data Entry: Current Season
- Authenticated form to record a new game result
- Select game, date, enter each player's score
- Create/select a season; supports populating the current season from scratch

### F4 — Leaderboard View
- Season leaderboard page (current season by default)
- Points aggregation query per player per season
- Season selector (click to switch seasons)

### F5 — Game History View
- List of games played in a season, ordered by date
- Basic game card: date, players, scores
- Linked from leaderboard

### F6 — Player Profile View
- Click a player → their profile page
- Per-season stats summary
- Full game history table across seasons

### F7 — Game Detail View
- Click a game → detailed view
- All players, scores, and outcome for that session
- Previous times this game was played (play history)

### F8 — Points Graph
- Line chart of each player's cumulative points over a season
- One line per player, x-axis = game number, y-axis = points
- Vanilla JS + canvas or a lightweight charting lib

### F9 — Data Entry: Historical Import
- Bulk entry for previous seasons
- Could be a simple CSV import or a multi-row form
- Needs to handle season creation as well

**Ordering note:** F1 → F2 → F3 → F4 is the critical path — gets the app functional with real data. F5–F9 can follow in any order.
