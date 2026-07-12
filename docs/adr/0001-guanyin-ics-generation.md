# ADR-0001: Guanyin ICS Generation

- **Status:** Proposed
- **Date:** 2026-07-13
- **Deciders:** Maintainer
- **Tags:** calendar, lunar, ics, guanyin, buddhism, offline

## Context

### Problem statement

We need a small Go program that, **entirely offline**, produces an iCalendar (`.ics`) feed of
the dates on which a Guanyin (觀音 / Avalokiteśvara) devotee should pray. The feed must cover the
**current year and the next two years**, must contain **Gregorian** dates (the input is lunar), and
must be served from memory by the **lightest possible HTTP server**. Each event must carry the
event name, the Gregorian date, and the local time window in which prayer is permitted.

The hard constraint is **accuracy**: the dates must match the dates a Chinese Buddhist temple
would publish. That requires correctly (a) enumerating the right set of lunar observances and
(b) converting lunar dates to Gregorian, which is non-trivial because the Chinese calendar is a
*lunisolar* calendar whose months and leap months are defined by astronomy, not by a fixed
arithmetic cycle.

### Research findings

#### 1. Which Guanyin dates require prayer?

The canonical set was cross-checked against two independent authoritative sources so that the
lunar month/day numbers and their meanings are unambiguous.

**Source A — `lunar-go` Buddhist festival database** (`FotoUtil.go`, `OTHER_FESTIVAL` map), the
de-facto Go reference for the Chinese Buddhist calendar:

| Lunar date | Entry in `OTHER_FESTIVAL` | Meaning |
|---|---|---|
| 2nd month, 19th day | `观世音菩萨圣诞` | Guanyin Bodhisattva's **Birthday** (圣诞 = holy birth) |
| 6th month, 19th day | `观音菩萨成道` | Guanyin Bodhisattva's **Enlightenment** (成道 = attaining the Way) |
| 9th month, 19th day | `观世音菩萨出家` | Guanyin Bodhisattva's **Renunciation** (出家 = leaving home to ordain) |

**Source B — Wikipedia, *Guanyin***: *"On the 19th day of the sixth lunar month, Guanyin's
attainment of Buddhahood is celebrated."* — independently corroborates the 6/19 enlightenment
date. Wikipedia, *Buddhist holidays* also lists *"Avalokiteśvara's Birthday … on the full moon
day in March"* — the March full moon is the Gregorian-side approximation of the 2nd lunar month's
15th/19th, again consistent with 2/19.

The "three nineteens" (三个十九) — **2/19, 6/19, 9/19** of the lunar calendar — are therefore the
three primary Guanyin commemoration days and are universally agreed across Chinese Buddhism.
(Note: an older, Taoist-leaning almanac table in the same library labels 9/19 as a "诞/birthday";
the Buddhist `OTHER_FESTIVAL` table, which is the religiously authoritative one, correctly labels
it "出家/renunciation". We follow the latter.)

#### 2. Recurring monthly observance days (朔望)

The 1st (初一, *shuo*, new moon) and 15th (十五, *wang*, full moon) of **every** lunar month are
traditional Buddhist/Taoist observance days (朔望斋, the *Uposatha* new- and full-moon fast days).
This is confirmed by:

- `lunar-go` `FotoUtil.FESTIVAL`, which attaches `月朔` (dj = "犯者夺纪", a major taboo) to every
  month's 1st and `月望` to every month's 15th.
- Wikipedia, *Buddhist holidays* → *Uposatha*: *"observance day … on the new moon, full moon,
  and quarter moon days every month."*

Several of these 1st/15th days are themselves major festivals and should be named as such rather
than as generic "初一/十五": 1/1 = **Spring Festival / Lunar New Year** (春节); 1/15 = **Lantern
Festival** (元宵); 7/15 = **Ghost Festival** (中元); 8/15 = **Mid-Autumn Festival** (中秋).

#### 3. Guanyin vegetarian fast days (观音斋) — optional extended set

`lunar-go` `FotoUtil.DAY_ZHAI_GUAN_YIN` lists 22 scattered Guanyin vegetarian/observance days:
`1-8, 2-7, 2-9, 2-19, 3-3, 3-6, 3-13, 4-22, 5-3, 5-17, 6-16, 6-18, 6-19, 6-23, 7-13, 8-16,
9-19, 9-23, 10-2, 11-19, 11-24, 12-25`. The three festival days (2/19, 6/19, 9/19) are a subset
of this list. We treat these as an *opt-in* secondary category (see Decision §Scope of events).

#### 4. How lunar dates are determined / converted to Gregorian

The Chinese calendar is a **lunisolar** calendar defined by the national standard
**GB/T 33661–2017** ("Calculation and Promulgation of the Chinese Calendar"), based on work by
the Purple Mountain Observatory. Its rules are fully algorithmic (i.e. computable offline, not
just from a table):

1. **Months begin on the day of the astronomical new moon** (geocentric conjunction). Each month
   is 29 or 30 days.
2. **The solar year (suì) is anchored on the December solstice.** The 24 solar terms are 15° steps
   of solar ecliptic longitude; the 12 *major* terms (中氣, *Zhōngqì*) sit at multiples of 30°.
3. **The winter solstice must fall in the 11th lunar month** — the central invariant.
4. **A lunar year with 13 lunations (i.e. 12 complete lunisolar months in one solar year) is a
   leap year.** The *first* lunisolar month that contains no major solar term is the leap month
   (閏), named for the month it follows (e.g. 閏六月).
5. **Chinese New Year** = the 2nd new moon after the previous winter solstice (3rd if the prior
   year had a leap 11th or 12th month). Equivalently, the new moon closest to *Lìchūn*
   (≈ Feb 4). It always falls in 21 Jan – 20 Feb.

Because new-moon and solar-term instants are timezone-sensitive, the conversion is defined
relative to **China Standard Time (UTC+8)**. The canonical English algorithmic reference is
Reingold & Dershowitz, *Calendrical Calculations* (4th ed., 2018), pp. 313–316.

Practical offline implementation strategies, in order of complexity:
- **(a) Astronomical computation** — compute new moons and the 24 solar-term instants from a solar
  & lunar ephemeris (e.g. Meeus' *Astronomical Algorithms*) and apply rules 1–5. Most accurate,
  no year-range limit, but requires an ephemeris implementation.
- **(b) Precomputed lookup tables** — embed a table of new-moon dates, month lengths, and leap
  positions for the years of interest, sourced from an official *Wànniánlì* (萬年曆). Simplest to
  reason about and to audit, but limited to the embedded year range.
- **(c) Hybrid (Reingold–Dershowitz fixed-cycle arithmetic)** — compact and deterministic, but can
  drift from the astronomical standard by a day at century scale.

#### 5. ICS (iCalendar) format

ICS (RFC 5545) is a plain-text line-oriented format. A feed is one `VCALENDAR` containing many
`VEVENT` blocks, each with `DTSTART`/`DTEND`, `SUMMARY`, `DESCRIPTION`, and `UID`. Timed events
use a `VTIMEZONE` block or a `TZID` parameter; all-day events use `VALUE=DATE`. No library is
strictly required — a correct emitter is ~60 lines of Go over a `strings.Builder`.

## Decision

### D1. Use `github.com/6tail/lunar-go` for lunar↔Gregorian conversion

We adopt **`github.com/6tail/lunar-go`** (MIT) as the **only** third-party dependency, pinned in
`go.mod`/`go.sum`.

- **Version:** `v1.4.6` — tag commit `dd82a5bc13f4e08417cb78b58f7a7fd90c1fd649`
- **License:** MIT (permissive, no copyleft concern)
- **Dependencies:** none (`go.mod` declares no requires) → fully offline, no network at build or
  runtime, no transitive supply-chain surface beyond the single module hash in `go.sum`.
- **CGO:** none — pure Go, static binary.
- **Why it is accurate:** its `ShouXingUtil` (授时历) implements the **astronomical** approach
  (strategy a above) — it computes new moons and solar terms from ephemeris formulae and applies
  the GB/T 33661–2017 rules, so it matches the official promulgated calendar for our target year
  range and well beyond. The README's own worked example (Lunar `1986-4-21` → Solar `1986-05-29`)
  is a verified public data point.
- **Bonus:** it ships the exact Buddhist festival tables we cross-checked our research against
  (`FotoUtil.OTHER_FESTIVAL`, `DAY_ZHAI_GUAN_YIN`, `IsDayZhaiShuoWang`), which doubles as a
  machine-checked reference for our date list.

**API surface we will use:**

```go
import "github.com/6tail/lunar-go/calendar"

// lunar (year, month, day) → Gregorian, leap months use negative month numbers (e.g. -6 = 閏六月)
lunar := calendar.NewLunarFromYmd(year, month, day)
solar := lunar.GetSolar()            // *Solar with GetYear/GetMonth/GetDay in UTC+8 Gregorian
```

We **do not** call the library's festival getters to drive event creation — we keep our own
explicit, documented date table (below) so the output is reviewable and the library is used only
for the *conversion* it is authoritative for. This keeps the religious-content decision in our
own audited code and the astronomy decision in the library.

### D2. Scope of events (what we generate)

For **each** of the current year and the next two years, generate one `VEVENT` per occurrence of:

**A. Guanyin commemoration days (3/year)** — fixed lunar month/day:

| Key | Lunar | `SUMMARY` (EN) | `SUMMARY` (ZH) | Note |
|---|---|---|---|---|
| `GUANYIN_BIRTH` | 2 / 19 | Guanyin Bodhisattva's Birthday | 观世音菩萨圣诞 | 圣诞 |
| `GUANYIN_ENLIGHTENMENT` | 6 / 19 | Guanyin Bodhisattva's Enlightenment | 观音菩萨成道 | 成道 |
| `GUANYIN_RENUNCIATION` | 9 / 19 | Guanyin Bodhisattva's Renunciation | 观世音菩萨出家 | 出家 |

**B. Monthly observance days (24/year, +2 when a leap month exists)** — 1st and 15th of every
lunar month, including the leap month. Special 1st/15th days are given their festival name
instead of the generic label:

| Lunar | Default `SUMMARY` | Override when festival |
|---|---|---|
| 1 / 1 | Lunar New Year (Spring Festival) — 春节 | — (always festival) |
| 1 / 15 | Lantern Festival (Shangyuan) — 元宵节 | — |
| 7 / 15 | Ghost Festival (Zhongyuan) — 中元节 | — |
| 8 / 15 | Mid-Autumn Festival — 中秋节 | — |
| other 1st | New-Moon Observance Day — 初一 (月朔) | — |
| other 15th | Full-Moon Observance Day — 十五 (月望) | — |

**C. Guanyin vegetarian fast days (观音斋) — opt-in via flag `--guanyin-zhai`**, default off.
When enabled, emit one event per day in `DAY_ZHAI_GUAN_YIN`, **deduplicated** against any
already-emitted A/B event on the same Gregorian day (the three festival days 2/19, 6/19, 9/19
overlap this list and must not produce duplicate `UID`s).

**Expected volume:** A + B ≈ **27–29 events/year** (24 monthly + 3 Guanyin, +2 in a leap year),
so ≈ **81–87 events over 3 years** — trivially small, generated and held in memory.

### D3. "Local times permitted to pray"

Each event is a **timed** `VEVENT` (not all-day), expressing the window in which prayer is
customarily permitted at a Guanyin temple:

- Default window: **05:00–21:00** on the Gregorian date (covers the morning 朝课 and the evening
  暮时诵经; Guanyin temples are generally open throughout the day). Configurable via
  `--pray-start 05:00` / `--pray-end 21:00`.
- Timezone: **`Asia/Shanghai` (UTC+8)** by default, because the lunar→Gregorian conversion is
  defined against CST. Configurable via `--tz`. The ICS embeds a `VTIMEZONE` block for the
  chosen zone so importing clients render correct local times regardless of the user's own
  timezone.
- `DTSTART`/`DTEND` carry a `TZID` parameter; `DTEND` is the `--pray-end` of the same Gregorian
  day.

### D4. ICS generation — hand-rolled, stdlib only

We write a minimal RFC 5545 emitter using only `strings.Builder` and the standard library. We do
**not** pull in an ICS library. Rationale: the output is a single calendar of simple timed
events; an ICS library would add a dependency for no functional gain; hand-rolling keeps the
dependency surface to exactly one module (`lunar-go`) and makes the output trivially auditable.

The emitter is responsible for:
- `VCALENDAR` header with `PRODID`, `VERSION:2.0`, `CALSCALE:GREGORIAN`.
- One `VTIMEZONE` for the configured zone (with at least one `STANDARD`/`DAYLIGHT` subcomponent
  taken from `time.LoadLocation`, so DST rules — if any — are correct).
- One `VEVENT` per occurrence: stable `UID` (`<category>-<lunarYear>-<lunarMonth>-<lunarDay>@guanyin-ics`),
  `DTSTART;TZID=…` / `DTEND;TZID=…`, `SUMMARY`, `DESCRIPTION` (English + Chinese name + the lunar
  date in words, e.g. "Lunar 2nd month, 19th day"), `STATUS:CONFIRMED`, `TRANSP:TRANSPARENT`.
- Line folding at 75 octets per RFC 5545 §3.1, UTF-8, CRLF line endings, and escaping of
  `,;\n` per §3.3.11.

The whole `[]byte` payload is computed **once at startup** (see D5) and served from memory.

### D5. HTTP server — `net/http`, single handler

A single `http.Handler` on `/:8080` (configurable via `--addr`) that returns the precomputed
`[]byte`:

- `GET /` and `GET /guanyin.ics` → `200 OK`, `Content-Type: text/calendar; charset=utf-8`,
  `Content-Disposition: attachment; filename="guanyin.ics"`, body = the in-memory bytes.
- Everything else → `404`.
- No logging of request bodies; no request parsing; no writes to disk; no outbound network.

The payload is built **once** in `main` (for the current year + next 2) and stored in a
`[]byte`. The handler is a closure over that byte slice; requests only do a `w.Write`. This is
the "lightweight simplest" design the task asks for: no framework, no middleware, no state.

**Recomputation:** at startup only. A 3-year window never changes during a process's lifetime
(the lunar calendar for those years is fixed). If long-running freshness is ever desired, the
design allows swapping the `[]byte` under a `sync.RWMutex` on a signal, but that is explicitly
out of scope for the "lightweight simplest" requirement.

## Consequences

**Positive**
- Single, pinned, MIT-licensed, zero-transitive-dep, pure-Go, offline module → minimal supply
  chain, static binary, trivial to vendor and audit.
- Accuracy is delegated to a library that implements the official astronomical rules (GB/T
  33661–2017) rather than to hand-rolled arithmetic, removing the most likely source of bugs.
- Event list is an explicit, reviewed table — the religious content is auditable in our own
  code, not hidden in a library's festival map.
- Output is plain text we generate ourselves, so the ICS is fully correct-by-construction and
  easy to diff/test.
- Server is a one-file `net/http` handler; binary has no runtime deps.

**Negative**
- A dependency on `lunar-go` for correctness means we must keep it pinned and re-verify on
  upgrades (mitigated by the verification harness in §Accuracy).
- Astronomical conversion is slightly heavier than a lookup table, but for ~87 events over 3
  years it is negligible (milliseconds once at startup).
- The "prayer time window" is a pragmatic default, not a religious rule; users in non-CST
  timezones who want true *local* prayer times must pass `--tz` (documented in `--help`).

**Neutral**
- We emit bilingual `SUMMARY` and a `DESCRIPTION`; clients that show only `SUMMARY` will display
  the English string (primary), which is the common convention.

## Alternatives considered

1. **Implement the lunisolar algorithm ourselves** (port Reingold–Dershowitz or a Meeus-based
   ephemeris). Rejected: high complexity, real risk of off-by-one-day bugs around new-moon /
   solar-term boundaries and the rare 2033-class leap-month anomaly, and we would be
   re-implementing exactly what `lunar-go` already does and tests.
2. **Embed a precomputed lookup table** (e.g. years 1900–2100 from a published *Wànniánlì*).
   Considered as a fallback. Pros: trivially auditable, zero compute. Cons: limited to the
   embedded range; we'd still need to source authoritative data and re-embed periodically. Not
   chosen as primary, but kept as a documented fallback if `lunar-go` is ever unavailable.
3. **Use an ICS library** (e.g. `arran4/go-ical`). Rejected: adds a dependency for a format we can
   emit correctly in ~60 lines; the hand-rolled emitter is smaller and more auditable.
4. **Use a web framework / middleware.** Rejected: `net/http` + one handler is the lightest
   correct option and the task explicitly asks for "lightweight simplest".
5. **Drive events from the library's own `GetOtherFestivals()`.** Rejected as the *source of
   truth* for our religious date list: we want the set of dates to be explicit and reviewable in
   our code. We *do* cross-check our table against that function in tests (see §Accuracy).

## Security considerations

- **No inbound data is trusted or persisted.** The handler ignores query strings and bodies and
  writes nothing to disk; there is no injection surface. Request paths are matched against a
  fixed allowlist (`/`, `/guanyin.ics`).
- **No outbound network** at runtime (the library is offline; `go.mod` has no network-bearing
  deps). The binary can run in a network-isolated container.
- **Supply chain:** one dependency, pinned by `go.sum` hash. We will run `govulncheck` in CI on
  every `go.mod` change and pin the Go toolchain version in `go.mod`'s `toolchain` directive.
- **DoS surface:** the response is a fixed-size in-memory `[]byte`; a `http.MaxBytesReader` is
  unnecessary because we never read request bodies. We set a generous `ReadTimeout`/`
  WriteTimeout` and `MaxHeaderBytes` on the `http.Server` to bound slowloris exposure.
- **No secrets, no PII** are processed or logged. Request logging, if added, is opt-in and
  redacts nothing-sensitive (there is nothing sensitive to redact).
- **Timezone loading** uses `time.LoadLocation`, which on stripped containers may need the
  `tzdata` package; we will `import _ "time/tzdata"` to embed the IANA TZ database into the
  binary so `Asia/Shanghai` resolves without the host having zoneinfo.

## Accuracy verification (mandatory, in the same repo)

Because correctness of dates is the central requirement, the implementation **must** include a
test harness that asserts conversion and event dates against **independently sourced** reference
data, so a library upgrade or a refactor cannot silently shift a date:

1. **Golden conversion tests** — for a spread of known years, assert `lunar.GetSolar()` for each
   Guanyin date and each 1st/15th matches a hardcoded expected Gregorian date sourced from a
   published almanac (e.g. the Hong Kong Observatory *Chinese calendar* tables, or the Purple
   Mountain Observatory published *Wànniánlì*). At minimum cover: the current year, a known leap
   year, and a year with a leap month that falls between two of our Guanyin dates (to prove the
   leap-month handling does not shift 2/19, 6/19, 9/19).
2. **Cross-check tests** — for every day our emitter produces a Guanyin-festival event, assert
   that `lunar.GetOtherFestivals()` for that Gregorian date contains the matching name
   (`观世音菩萨圣诞` / `观音菩萨成道` / `观世音菩萨出家`). This catches any drift between our
   explicit table and the library's tables.
3. **ICS validity tests** — parse the generated `[]byte` back with an independent parser in tests
   and assert: RFC 5545 line-folding, CRLF endings, `UID` uniqueness, `DTSTART < DTEND`, and
   that every `DTSTART`'s date equals the `lunar.GetSolar()` date.
4. **Reference date fixture (example):** Guanyin's Birthday, Lunar 2024 / 2 / 19 → expected
   Gregorian **2024-03-28** (to be confirmed against an almanac and pinned in the test; the
   harness fails loudly if `lunar-go` ever disagrees).

## References

- Wikipedia, *Guanyin* — confirms 6/19 enlightenment commemoration.
- Wikipedia, *Buddhist holidays* — *Avalokiteśvara's Birthday*; *Uposatha* (new/full-moon
  observance days).
- Wikipedia, *Chinese calendar* — lunisolar rules; GB/T 33661–2017; winter-solstice-in-11th-month
  invariant; leap-month = first month without a major solar term.
- Wikipedia, *Public holidays in China* — confirms lunar dates of Spring Festival (1/1),
  Lantern Festival (1/15), Ghost Festival (7/15), Mid-Autumn (8/15).
- `github.com/6tail/lunar-go` `FotoUtil.go` — `OTHER_FESTIVAL` map (authoritative Buddhist
  festival dates incl. the three Guanyin "nineteens") and `DAY_ZHAI_GUAN_YIN` (Guanyin fast days).
- Reingold & Dershowitz, *Calendrical Calculations*, 4th ed., 2018, pp. 313–316 — canonical
  English algorithm for the Chinese calendar.
- GB/T 33661–2017, *Calculation and Promulgation of the Chinese Calendar* (Standardization
  Administration of China).
- RFC 5545, *Internet Calendaring and Scheduling Core Object Specification (iCalendar)*.
