# usc

A command-line client for USC's Brightspace, Web Registration, Handshake, Leavey Library rooms, and the public Schedule of Classes. Every command prints JSON, so Claude Code, Codex, or any agent with a terminal can run it for you.

Docs: [usc.sh/docs](https://usc.sh/docs/)

## Install

Paste this into your agent:

```text
Set up the USC CLI for me: https://usc.sh/install.md
```

Or by hand, on macOS and Linux:

```sh
curl -fsSL https://usc.sh/install | sh
```

With Go 1.24 or later, on any platform:

```sh
go install github.com/nsigel/usc-cli/cmd/usc@latest
```

## Sign in

```sh
usc auth login
```

It asks for your NetID, password, and a [Duo bypass code](https://account.usc.edu/2fa/duo-bypass-code), and saves them on your machine (`0600`). Every later command signs itself in. For scripts and CI, set `USC_USERNAME`, `USC_PASSWORD`, and `USC_DUO_BYPASS` instead. Details: [Signing in](https://usc.sh/docs/auth/).

## Give your agent the skill

`usc skill` prints a SKILL.md that teaches an agent every command. For Claude Code:

```sh
mkdir -p ~/.claude/skills/usc-cli
usc skill > ~/.claude/skills/usc-cli/SKILL.md
```

## Examples

```sh
usc brightspace courses
usc brightspace announcements --since 2026-11-09T00:00:00-08:00
usc webreg courses --term 20271 --course BUAD-306
usc handshake career-fairs --date next-30
usc libcal schedule --date tomorrow --category rooms --min-capacity 4
usc classes 20271 CSCI-270
```

## Commands

| Command | |
| --- | --- |
| `usc auth login` / `status` / `logout` | Sign in, check the session, or delete it |
| `usc brightspace whoami` | Your Brightspace user |
| `usc brightspace courses [--all]` | Enrolled classes; `--all` adds departments, semesters, and groups |
| `usc brightspace content COURSE_ID [--flat]` | A course's content tree |
| `usc brightspace grades COURSE_ID [--graded-only]` | Grades for a course |
| `usc brightspace announcements [COURSE_ID] [--since TIME]` | Announcements for one course or all of them |
| `usc brightspace assignments COURSE_ID` | Assignment folders |
| `usc brightspace submissions COURSE_ID FOLDER_ID [--download] [--output DIR]` | Your submissions, or download the files |
| `usc brightspace download COURSE_ID TOPIC_ID [--output DIR]` | Download a content file |
| `usc webreg terms` | Registration terms and their codes |
| `usc webreg programs --term TERM` | Schools and program codes |
| `usc webreg courses --term TERM (--program CODE \| --course CODE)` | Courses with sections, seats, and clearance |
| `usc webreg bin --term TERM` | Your course bin |
| `usc webreg calendar --term TERM` | Weekly meetings, with conflicts marked |
| `usc webreg add\|schedule\|unschedule\|remove SECTION_ID --term TERM` | Plan course-bin changes |
| `usc webreg grade-option SECTION_ID CODE --term TERM` | Change a grade option |
| `usc webreg checkout --term TERM` | Preview what would be registered and dropped |
| `usc webreg submit --term TERM --confirm` | Register and drop what checkout shows |
| `usc handshake categories` | Event categories |
| `usc handshake events [FILTERS]` | Search events |
| `usc handshake event EVENT_ID` | One event |
| `usc handshake career-fairs [FILTERS]` | Search career fairs |
| `usc handshake career-fair CAREER_FAIR_ID` | One career fair, with sessions |
| `usc libcal categories` | Leavey room and pod categories |
| `usc libcal spaces [rooms\|pods\|all\|lvl1\|lvl2\|lvl3]` | Leavey spaces |
| `usc libcal space SPACE_ID` | One space |
| `usc libcal schedule [FILTERS]` | Open and taken times for each space |
| `usc libcal book --space ID --start TIME --end TIME --accept-terms` | Book exactly that space and time |
| `usc libcal release` | Clear an interrupted booking |
| `usc classes TERM_CODE COURSE_CODE` | Public course and sections, no sign-in |
| `usc browser sync [--cdp TARGET]` | Copy the session into Chrome |
| `usc skill` | Print the agent skill |
| `usc version` | Print the version |

Filters, IDs, and the registration and booking flows are in the [docs](https://usc.sh/docs/).

## Output

Commands print JSON to stdout: indented in a terminal, one line when piped. `--pretty` and `--compact` override that. Errors are JSON on stderr with exit status 1, and name the command that fixes them when there is one:

```json
{"error":"brightspace: USC sign-in is required","action":"usc auth login"}
```

## Go library

Every command is a call into a package you can use directly:

```go
client, err := brightspace.Open(ctx, auth.Options{CredentialsFile: file})
courses, err := client.Courses(ctx)
```

See [Go library](https://usc.sh/docs/library/) and [`examples/`](examples/).

## Website

The site at [usc.sh](https://usc.sh) is in [`site/`](site/): `npm install && npm run dev` to work on it.

## Contributing

Issues and pull requests are welcome.

## License

MIT. Not affiliated with USC.
