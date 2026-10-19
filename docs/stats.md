# Server stats

What the server has and what it is spending, so people can see what their
pictures and videos add up to. Server: [`stats.go`](../server/stats.go) and the
platform files beside it. App: the Server tab,
[`(tabs)/server.tsx`](../mobile/src/app/(tabs)/server.tsx).

## GET /api/stats

Session required. Answers:

```json
{
  "storage": {
    "disk_total": 494384795648, "disk_free": 85396652032,
    "uploads": 7340032, "database": 1048576,
    "yours": 2621440, "upload_limit": 20000000000
  },
  "cpu": { "cores": 8, "load_1": 0.42, "load_5": 0.31, "load_15": 0.25, "server_percent": 1.4 },
  "memory": { "total": 16777216000, "available": 9663676416, "server": 48234496 }
}
```

- **storage**: `disk_total` and `disk_free` describe the volume the media
  directory sits on (`statfs`; free is what ordinary programs may still use).
  `uploads` is the size of everything in the media directory, `database` what
  MariaDB reports for the tables and indexes (`information_schema`). `yours` is the uploads the signed-in
  person's posts, messages, profile picture and cover reference, each file
  counted once however often it appears (`userStorage`). `upload_limit` is
  the most one upload may be ([media.md](media.md#post-apimedia)).
- **cpu**: `cores` from the Go runtime; `load_1`, `load_5` and `load_15` are
  the system load averages, the number of runnable processes averaged over
  those minutes, so a load equal to `cores` means every core is busy.
  `server_percent` is this process's own CPU use as a share of one core,
  measured between two samples of its CPU time taken every five seconds
  (`meterCPU`).
- **memory**: `total` and `available` for the machine, where available is
  free memory plus what the OS would reclaim from caches for a program that
  asks. `server` is the memory the Go runtime holds for this process, close
  to what the OS shows as resident (`runtime/metrics`).

Where the figures come from depends on the platform, through
`golang.org/x/sys/unix`:

| Platform | Disk and process CPU | Memory and load |
| --- | --- | --- |
| Linux (`stats_linux.go`) | `statfs`, `getrusage` (`stats_unix.go`) | `sysinfo` for the total and the load averages, `MemAvailable` from `/proc/meminfo`. |
| macOS (`stats_darwin.go`) | the same | `sysctl`: `hw.memsize`; `kern.memorystatus_level`, the percentage its memory-pressure accounting counts as available (macOS keeps the rest busy with caches on purpose), else the free and speculative page counts times the page size; `vm.loadavg`. |
| Anything else (`stats_other.go`) | 0 | 0 |

A 0 means unknown; the app says so instead of drawing an empty bar. The
figures describe the host to every signed-in user, which is the point: a
shared server's disk is everyone's business. Nothing about other people is
included.

## The Server tab

The tab loads the stats when shown and every ten seconds while it stays on
screen, since load and memory move; pull down to reload at once. Three cards:

- **Storage**: a bar for your share of everyone's uploads, then the disk
  (used of total, red past 90 %), and rows for everyone's uploads, the
  database and the largest upload allowed.
- **Processor**: the one-minute load against the number of cores (red past
  90 %), the five- and fifteen-minute figures, and this server's own share of
  one core.
- **Memory**: in use of total (red past 90 %), what is available, and what
  this server holds.

Sizes use decimal units (1 GB = 1 000 000 000 bytes), the way phones and
browsers show file sizes; the upload limit is set in the same units.
