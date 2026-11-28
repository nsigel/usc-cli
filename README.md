# usc

A command-line client for USC's Brightspace, Web Registration, Handshake, the Student Directory, Leavey Library rooms, USC mobile ordering, and the public Schedule of Classes. It signs in with your NetID and Duo, then reuses its own Duo device so later commands need no prompts. Every command prints JSON, so Claude Code, Codex, Grok, or any agent with a terminal can run it for you.

Docs: [usc.sh/docs](https://usc.sh/docs/)

## Install or update

Ask your agent:

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

To update `usc` and its skill, ask your agent again. By hand, rerun the install command and regenerate the skill (see [Give your agent the skill](#give-your-agent-the-skill)).

## Sign in

```sh
usc auth login --web
```

Open the printed `url` and sign in with your NetID, password, and Duo. Later commands authenticate automatically. On remote machines, use `usc auth login --remote`; for scripts and CI, set `USC_USERNAME`, `USC_PASSWORD`, and `USC_DUO_TOTP_SECRET` or `USC_DUO_BYPASS`. Details: [Sign in](https://usc.sh/docs/auth/).

## Give your agent the skill

`usc skill` prints a SKILL.md with the commands and the rules an agent follows, such as asking before it registers, books, or orders. Ask your agent to "add the usc skill: save the output of `usc skill` as a skill named usc-cli", or for Claude Code:

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
usc directory students --last-name smith --school Viterbi
usc classes 20271 CSCI-270
```

## Docs

Each page gives the commands, filters, and flows for one area:

- [Sign in](https://usc.sh/docs/auth/)
- [Brightspace](https://usc.sh/docs/brightspace/)
- [WebReg](https://usc.sh/docs/webreg/)
- [Handshake](https://usc.sh/docs/handshake/)
- [Student Directory](https://usc.sh/docs/directory/)
- [Leavey Library](https://usc.sh/docs/libcal/)
- [Mobile ordering](https://usc.sh/docs/mobileorder/)
- [Schedule of Classes](https://usc.sh/docs/classes/)
- [Output and errors](https://usc.sh/docs/output/)
- [Go library](https://usc.sh/docs/library/), with [`examples/`](examples/)

`usc --help` and `usc skill` also list the commands.

## Website

The site at [usc.sh](https://usc.sh) is in [`web/`](web/): `npm install && npm run dev` to work on it.

## Contributing

Issues and pull requests are welcome.

## License

MIT. Not affiliated with USC.
