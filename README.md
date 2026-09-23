# sabtop

A terminal console for [SABnzbd](https://sabnzbd.org): the download queue,
history, disk headroom and news-server stats, without opening a browser.

```
sabtop  SABnzbd 5.1.0 · http://127.0.0.1:8080                                       10:27:16
 1 Queue   2 History   3 Status   4 Servers

  JOB                                              CATEGORY SIZE      LEFT                    ETA
▼ Ancient.Aliens.S01E01.2160p.REMUX-GRP            tv       77.4 GB   43.3 GB  ██████░░░ 44%  0:13:40
· Deep.Sea.Rescue.2019.UHD.BluRay.REMUX-GRP        movies   76.9 GB   76.9 GB  ░░░░░░░░░ 0%   0:37:59
· Quiet.Harbour.1962.UHD.2160p.REMUX-GRP           movies   55.7 GB   55.7 GB  ░░░░░░░░░ 0%   0:55:35
⏸ Night.Market.1988.1080p.BluRay.Remux-GRP         movies   26.9 GB   26.9 GB  ░░░░░░░░░ 0%   —
! Broken.Release.2011.2160p.REMUX-GRP              movies   49.6 GB   12.1 GB  ███████░░ 75%  0:04:11

downloading · 160 jobs · 4.59 TB left · 56.6 MB/s · eta 23:36:18 · 20 warnings
space pause/resume job · P pause/resume all · +/- priority · L speed limit · x delete
```

## Why

SABnzbd's web UI is fine until something goes wrong. Then it tells you
**"Too little diskspace forcing PAUSE"** — and never which volume. If your
categories write to absolute paths on different drives, that warning sends you
hunting. sabtop's Status tab names the volume.

## What it does

| Tab | Shows | Actions |
|---|---|---|
| **Queue** | every job, progress, size remaining, ETA, missing articles | pause/resume one or all, re-prioritise, delete, set a speed limit |
| **History** | completed, failed and post-processing jobs, with the failure message | filter to failures, retry, delete |
| **Status** | **every volume SABnzbd writes to**, the queue state, the safety switches, recent warnings | pause/resume, clear warnings |
| **Servers** | per-news-server usage for today/week/month/all time, articles fetched | — |

Every action that changes server state asks first.

### The disk tab is the point

SABnzbd's API reports free space for exactly two paths: the temp dir and
`complete_dir`. A category configured with an **absolute** `dir` can live on a
third volume that SABnzbd never measures — and when that one fills, the queue
stops with a warning naming no path at all.

sabtop lists those category volumes too, stat-ing them directly, and flags any
that fall below SABnzbd's own `download_free` / `complete_free` floor or under
10% headroom:

```
Disks
  temp         ████░░░░░░░░░░░░░░░░░░░░  16.8%  198 GB free of 238 GB
               /Volumes/fast-ssd/usenet/incomplete
  complete     ████████████████████░░░░  83.6%  2.11 TB free of 12.92 TB
               /Volumes/nas-a/downloads/complete
  cat:movies   ████████████████████████ 100.0%  0 GB free of 17.92 TB   ← below SAB's 2G floor, downloads will pause
               /Volumes/nas-b/downloads/complete/movies
```

This needs sabtop to see the same mounts as SABnzbd, which is the usual case.
Where it can't, the row says so rather than inventing a number.

## Install

```sh
go install github.com/NilssonMandola/sabtop@latest
```

## Setup

sabtop needs SABnzbd's API key, from **Config → General → API Key**:

```sh
export SAB_URL=http://127.0.0.1:8080
export SAB_API_KEY=<key>
sabtop
```

…or write `~/.config/sabtop/config`:

```ini
url   = http://127.0.0.1:8080
token = <key>
```

Flags beat the environment, which beats the config file:

```sh
sabtop -url http://nas:8080 -key <key>
```

## Keys

| Key | Does |
|---|---|
| `1`…`4` | jump to a tab |
| `tab` / `shift+tab` | cycle tabs |
| `j` / `k`, arrows | move the cursor |
| `g` / `G` | top / bottom |
| `r` | refresh now |
| `?` | help |
| `q` | quit |

Per tab: `space` pause/resume a job, `P` pause/resume everything, `+`/`-`
priority, `L` speed limit, `x` delete (Queue) · `enter` details, `f` failures
only, `R` retry, `x` delete (History) · `P` pause/resume, `C` clear warnings
(Status).

## Notes and limits

- **"Downloading" means nothing in SABnzbd's queue.** Every queued job carries
  that status, including the 159 that have not started. sabtop marks activity
  by progress instead: `▼` has bytes on disk, `·` is still waiting.
- **Category volumes need local mounts.** See above.
- **History is the last 200 entries.** SABnzbd pages further back; sabtop does
  not yet.
- Tested against SABnzbd 5.1.

## License

MIT — see [LICENSE](LICENSE).
