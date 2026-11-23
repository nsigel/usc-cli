---
layout: ../../layouts/Docs.astro
title: Student Directory
description: Search the USC Student Directory by name, major, school, email, or phone.
---

```sh
usc directory students --query TEXT
usc directory students [--first-name NAME] [--last-name NAME] [--major TEXT] [--school TEXT] [--email TEXT] [--phone TEXT]
usc directory student STUDENT_ID
```

Reads the [USC Student Directory](https://uscdirectory.usc.edu/web/directory/student/), which needs a USC sign-in.

## Search

`--query` is the directory's basic search over name, major, and email. The other flags are its advanced search, and you can combine them with each other. You can't combine them with `--query`, because the directory ignores the basic search when any field is set. Every value matches as a substring:

```sh
usc directory students --query smith
usc directory students --last-name smith --school Viterbi
```

```json
{
  "students": [
    {
      "id": "scfj7mn6",
      "name": "Kaitlyn Smith",
      "full_name": "Kaitlyn Smith",
      "email": "smithks@usc.edu",
      "major": "English (Creative Writing) (CRWT)",
      "degree_program": "Bachelor of Science (BS)",
      "school": "Andrew and Erna Viterbi School of Engineering (ENGR)"
    }
  ],
  "truncated": false
}
```

The directory returns at most 100 matches. When `truncated` is `true`, more students may match, so narrow the search.

Fields a student hasn't published are left out. `title`, `department`, and `division` show up only for students with a USC job, and some entries also have `phone`, `mail_code`, and `address` (a list of lines).

## One student

```sh
usc directory student scfj7mn6
```

`STUDENT_ID` is an `id` from a search.
