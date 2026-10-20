# Scoring versions

The score is a number from 0 to 100 for how densely a text carries the patterns the active
profile lists. Builds gate on it, so it has a version. This page says what the version
promises, how to pin a gate to it, and what has moved so far.

## The promise

Every score names the scoring version that produced it, as `scoringVersion` in
`score --json`, `fix --json`, the MCP tools, and the web engine. Within one version, the
same text under the same profile, presets, and dialect gets the same score. The version rises
by one on any release that can change the score of unchanged text:

- A signal is added, removed, or reweighted.
- A threshold moves.
- A rule that carries score weight is added to or removed from the default profile or a
  built-in preset.

A release that touches none of these keeps the number. Whitespace and punctuation tidy-ups
carry no weight, and neither do the editor integrations, the web app, and the docs.

## How the repo keeps the promise

`sanitize/testdata/scoring.golden.json` pins the score of every passage in the benchmark
corpus under the default profile and each built-in preset. `TestScoringContract` fails when
any of them moves without a bump of `ScoringVersion` in `sanitize/score.go`, and the update
flag refuses to rewrite the file under the same version. The diff of that file in a pull
request is the regression report, one line per passage, naming every profile it moved under.

The test sees what the corpus exercises. A new block word that no corpus passage contains
moves nothing the test can measure, so the bump rule above binds the person making the
change and not only the build.

## Pin a gate

Set the action's `scoring-version` input to the version your gate was tuned against:

```yaml
with:
  files: docs/intro.md
  max: 25
  scoring-version: "1"
```

When a newer tag scores at a different version, the job fails before it scores anything and
points here. A tag bumper such as Dependabot then stops for a human instead of moving your
gate. The tag pins the engine as well, so a pinned tag scores tomorrow as it does today.

On the command line the version rides along in the JSON:

```sh
slop-chop score --json notes.md | jq .scoringVersion
```

Compare scores only within one version. A score of 31 under version 1 and a score of 31 under
version 2 are two different statements about the text.

## What moved

| Version | Release | What moved                                                                                                                                                                                                                                                                                      |
|---------|---------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 3       | v0.52.0 | Four guards for text that is not an English sentence: anaphora on list items and HTML rows, template stems on colon lead-ins and list items, the idiom `may or may not` and the month May in hedge stacks, and em dashes doubled, between digits, or on a CJK line. No benchmark passage moves. |
| 2       | v0.49.0 | Three standard library false positives: a sentence restated word for word, #include lines in a comment, and Greek math symbols.                                                                                                                                                                 |
| 1       | v0.48.0 | The first stamped version. It scores exactly as v0.47.0 did, which the benchmark corpus confirms: none of its 153 passages moves between the two.                                                                                                                                               |

### Before the stamp

Releases before v0.48.0 carry no version. To reconstruct them, each tag from v0.31.0 to
v0.47.0 was built and used to score the benchmark corpus as it stands today, under the
default profile. Six of those 23 releases moved at least one score. Every release not listed
moved none, which says nothing about text the corpus lacks.

| Release | Passages moved | Largest move | What shipped                                                                                     |
|---------|----------------|--------------|--------------------------------------------------------------------------------------------------|
| v0.44.0 | 6              | 77           | The clause habit, and the rules for `ensure`, the announce flourish, and happy sign-offs.        |
| v0.43.0 | 1              | 5            | The participle tail habit.                                                                       |
| v0.41.0 | 75             | 15           | Flat cadence and accumulated evidence.                                                           |
| v0.39.0 | 2              | 79           | The landing drumbeat, scored by its rate.                                                        |
| v0.38.0 | 7              | 80           | The copula landing and the stem turned against itself, two sentence shapes the word list misses. |
| v0.34.0 | 2              | 32           | The weighing register, connectors that present every side and land nowhere.                      |

## Changing the scoring

Change the engine, then run `go test ./sanitize -run TestScoringContract`. A failure that
names moved scores means the version has to rise. Raise `ScoringVersion` in
`sanitize/score.go`, then regenerate the pinned file:

```sh
go test ./sanitize -run TestScoringContract -update-scoring
```

Add a row to the table above naming what moved, and commit the pinned file with the change.

Then rerun the human-prose measurement in [the benchmark](BENCHMARK.md), both columns,
since the scoring contract covers neither. The two README corpora stay local, and
`evaldata/README.md` says how to collect them:

```sh
go run ./evaldata/harness -pre2022
go run ./evaldata/harness -pre2022 -pre2022-file evaldata/pinned.jsonl
```

A moved count changes the benchmark table, the README, and POSITIONING.md together.
