# Switch Tester 5000

A cross-platform terminal UI tool for measuring per-port degradation on
unmanaged network switches. Two computers send UDP traffic through the switch
under test and measure packet loss at progressively higher bandwidths, giving
you a precise picture of which ports are failing and how badly.

## Why this exists

Unmanaged switches don't expose telemetry. A port can be quietly dropping 20%
of packets at 100 Mbps while appearing "connected" in every other tool. This
tool finds those faults by treating each port as a black box and stress-testing
it with real traffic.

---

## Physical setup

```
  ┌──────────────────┐        ┌──────────────────┐
  │ Measuring        │        │ Mirroring        │
  │ Computer         │        │ Computer         │
  └────────┬─────────┘        └─────────┬────────┘
           │ Ethernet                   │ Ethernet
           │                            │
     ┌─────▼────────────────────────────▼─────┐
     │         Switch under test              │
     │  port 1       port 2 … port N          │
     └────────────────────────────────────────┘
```

- Both computers must be on the **same Layer 2 network** (no router between them and the switch).
- The **measuring computer** stays on a fixed port throughout the test.
- The **mirroring computer** moves to each other port in turn, one at a time.
- You only need **one cable move per port** — the app tells you exactly when and where.

---

## Quick start

### Download a pre-built binary

Go to the [Releases](../../releases) page and download the binary for your platform:

| File | Platform |
|---|---|
| `switch-tester-5000-linux-amd64` | Linux x86-64 |
| `switch-tester-5000-linux-arm64` | Linux ARM64 / Raspberry Pi (64-bit) |
| `switch-tester-5000-darwin-amd64` | macOS Intel |
| `switch-tester-5000-darwin-arm64` | macOS Apple Silicon |
| `switch-tester-5000-windows-amd64.exe` | Windows x86-64 |

On Linux/macOS, make it executable:

```sh
chmod +x switch-tester-5000-linux-amd64
./switch-tester-5000-linux-amd64
```

### Build from source

Requires Go 1.22+.

```sh
git clone https://github.com/jmcateer/switch-tester-5000
cd switch-tester-5000
make run          # run directly
make all          # cross-compile all platforms into dist/
make linux        # linux amd64 + arm64 only
make windows      # windows amd64 only
make macos        # darwin amd64 + arm64 only
```

---

## Running a test

### Step 1 — start the mirroring computer first

On the computer that will be moving between switch ports, launch the binary and
select **Mirroring Computer**. Enter the UDP port to listen on (default: 5000)
and press **Start Mirroring**. Leave this window open for the duration of the
test.

The mirroring computer echoes every valid packet it receives back to the sender.
It shows live stats (packets received, echoed, bytes, uptime) but requires no
other interaction.

### Step 2 — configure the measuring computer

On the computer that stays on a fixed port, launch the binary and select
**Measuring Computer**. Fill in the setup form:

| Field | What it means |
|---|---|
| Mirror Computer IP | IP address of the mirroring computer |
| Mirror UDP Port | Port the mirror is listening on (must match) |
| Listen (reply) Port | Local port the measuring computer binds for receiving echo replies |
| Switch Name | Label used in instructions and results (e.g. "TP-Link TL-SG108") |
| Switch Port Count | Total number of ports on the switch |
| Measuring Computer Port # | Which port the measuring computer is plugged into |
| Loss Threshold % | Packet loss percentage that counts as a "fail" (default: 25%) |
| Test Duration (sec) | How long to send at each bandwidth level (default: 3 s) |
| Packet Size (bytes) | UDP payload size (default: 1024 bytes) |

All settings are saved to a TOML config file next to the binary on **Save &
Continue**, so you only fill this in once.

### Step 3 — follow the cable instructions

The app generates step-by-step cable instructions. For the first test it tells
you where to plug both computers. After each port test, it tells you exactly
which port to move the mirroring computer's cable to. The measuring computer
never moves.

For an 8-port switch with the measuring computer on port 1, the sequence is:

```
Test 1: mirror on port 2  →  test 2  →  move mirror to port 3
Test 2: mirror on port 3  →  test 3  →  move mirror to port 4
...
Test 7: mirror on port 8  →  done
```

### Step 4 — run each bandwidth level test

Press **Begin Test** after moving the cable. The app:

1. Pings the mirror to confirm connectivity (fails fast if the cable isn't seated)
2. Sends UDP packets starting at 500 Kbps, doubling every level: 500K → 1M → 2M → 4M → 8M → …
3. Each level runs for the configured duration, then measures what fraction of
   packets the mirror echoed back
4. Stops the ladder when loss exceeds the threshold, or at 1 Gbps

A live progress bar shows the receive percentage at each level as it runs.

### Step 5 — read the results

After all ports are tested, the results table shows every port against every
bandwidth level tested:

```
┌──────┬──────┬──────┬──────┬──────┬──────┬──────┬──────┬──────────┐
│ Port │ 500K │  1M  │  2M  │  4M  │  8M  │ 16M  │ Max BW│ Status  │
├──────┼──────┼──────┼──────┼──────┼──────┼──────┼──────┼──────────┤
│  2   │  ✓   │  ✓   │  ✓   │  ✓   │  ✓   │  ✗   │  8M  │ Degraded │
│  3   │  ✓   │  ✓   │  ✓   │  ✓   │  ✓   │  ✓   │ 16M+ │  Good    │
│  4   │  ✓   │  ✗   │  —   │  —   │  —   │  —   │ 500K │  Failed  │
└──────┴──────┴──────┴──────┴──────┴──────┴──────┴──────┴──────────┘
```

- **✓** = passed (loss ≤ threshold)
- **✗** = failed (loss > threshold) — ladder stops here for this port
- **—** = not tested (port already failed at a lower level)

**Status categories:**

| Status | Meaning |
|---|---|
| Good | Maximum passing bandwidth ≥ 10 Mbps |
| Degraded | Passes some levels but max < 10 Mbps |
| Failed | Drops too many packets even at 500 Kbps |

The summary below the table gives aggregate counts and an overall health
verdict.

---

## Configuration file

The config file is named `<binary-name>.toml` and lives in the **same directory
as the binary**. It is created automatically when you first press Save &
Continue on the measuring computer, and on the mirroring computer when you
start mirroring. It is gitignored by default.

Full annotated example:

```toml
[network]
listen_port = 5001      # local port the measuring computer binds (reply socket)
mirror_ip   = "192.168.1.50"
mirror_port = 5000      # port the mirroring computer listens on

[test]
start_bandwidth_kbps   = 500    # first rung of the bandwidth ladder
loss_threshold_percent = 25.0   # % loss that counts as a test failure
test_duration_seconds  = 3      # seconds to send at each bandwidth level
packet_size_bytes      = 1024   # UDP payload size in bytes

[switch]
port_count   = 8
measure_port = 1        # port the measuring computer is always on
switch_name  = "TP-Link TL-SG108"
```

**Note:** `listen_port` and `mirror_port` must be different when both computers
are on the same machine (useful for local testing). In normal use on separate
machines they can both be 5000.

---

## Firewall notes

Both computers must be able to exchange UDP packets on the configured ports.

- **Mirroring computer:** allow inbound UDP on `mirror_port` (default 5000)
- **Measuring computer:** allow inbound UDP on `listen_port` (default 5001)

On Linux with `ufw`:

```sh
ufw allow 5000/udp   # mirroring computer
ufw allow 5001/udp   # measuring computer
```

On Windows, add an inbound rule for UDP on the relevant port in Windows
Defender Firewall.

---

## Interpreting results

### What the numbers mean

The bandwidth ladder doubles each step, so the results give a logarithmic view
of port health. A port that fails at 2 Mbps on a nominally 1 Gbps switch is
losing roughly 500× its rated capacity to degradation. Common failure modes:

- **Fails at all levels** — damaged RJ45 jack, corroded contacts, bent pins, or
  a failed PHY on the switch IC
- **Fails above ~10 Mbps** — marginal cable quality, partial contact failure,
  or EMI sensitivity on a worn port
- **Passes all levels but shows elevated loss** — intermittent contact,
  firmware-level queuing issue, or end-of-life buffer degradation

### Limitations

- The test measures the **path** through the switch, not an isolated port. The
  measuring computer's port (port 1 by default) is always in the path and is
  not independently characterized. If you suspect it is the failing port, swap
  the measuring and mirroring roles and re-run.
- UDP loss at high rates can also be caused by the **receiving computer's OS
  buffer** being too small, not the switch. If all ports fail at the same
  threshold, try increasing the OS UDP receive buffer
  (`net.core.rmem_max` on Linux) or reducing `packet_size_bytes`.
- The tool uses software timers for rate control. At very high bandwidths
  (above ~100 Mbps) the host OS scheduler may introduce jitter. Results above
  100 Mbps are indicative rather than precise.

---

## Developer guide

### Repository layout

```
switch-tester-5000/
├── main.go                        Entry point: resolves config path, starts TUI
├── version.txt                    Single source of truth for the version number
├── Makefile                       Cross-compilation targets
├── create-release.py              Bump version, tag, push — triggers CI release
├── .github/
│   └── workflows/
│       └── release.yml            Build + publish on v* tags
└── internal/
    ├── config/
    │   └── config.go              Config struct, Load/Save (BurntSushi/toml)
    ├── network/
    │   ├── protocol.go            UDP packet layout: magic + seq + timestamp_ns
    │   ├── mirror.go              Echo server (mirroring computer)
    │   └── sender.go              Rate-controlled sender + Ping (measuring computer)
    └── tui/
        └── app.go                 Full interactive TUI (rivo/tview + gdamore/tcell)
```

### Key design decisions

**Config file location** — `main.go` resolves symlinks on the executable path
before computing the config filename. This means the config follows the binary
even when invoked via a symlink from `/usr/local/bin`.

**Rate control** — `sender.RunBandwidthLevel` uses a time-based token approach:
it computes `targetPackets = elapsed_seconds * packetsPerSecond` and sends
bursts to catch up if behind. This works reliably from 500 Kbps to 1 Gbps
without ticker resolution problems. A brief sleep (capped at 1 ms) prevents
100% CPU use when the sender is ahead of schedule.

**Loss measurement** — sent sequence numbers are stored in a `map[uint64]struct{}`.
When an echo arrives, the sequence is looked up and deleted. After the send
phase ends there is a 600 ms drain window before the receiver goroutine is
stopped, giving in-flight echoes time to arrive.

**Thread safety** — all TUI state is owned by the main goroutine. Goroutines
spawned for mirroring stats updates and test execution call
`tvApp.QueueUpdateDraw(func)` to push changes back to the UI thread. The mirror
server uses `sync/atomic` counters so stats reads are race-free without a mutex.

**Page management** — `switchTo(name, primitive)` removes and re-adds the named
page each time. This means pages are rebuilt from current state on every
navigation, keeping the code simple at the cost of a tiny allocation per
navigation event.

### Dependencies

| Package | Version | Purpose |
|---|---|---|
| `github.com/BurntSushi/toml` | v1.6+ | Config file read/write |
| `github.com/rivo/tview` | v0.42+ | Terminal UI widgets |
| `github.com/gdamore/tcell/v2` | v2.13+ | Terminal rendering / color constants |

No CGO, no OS-specific code — `CGO_ENABLED=0` cross-compilation works out of
the box for all targets.

### Packet format

Every UDP datagram begins with a 20-byte header (big-endian):

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
├───────────────────────────────────────────────────────────────────┤
│                    Magic = 0x53574954 ("SWIT")                    │  bytes 0–3
├───────────────────────────────────────────────────────────────────┤
│                     Sequence number (uint64)                      │  bytes 4–11
├───────────────────────────────────────────────────────────────────┤
│                  Send timestamp (int64, nanoseconds)              │  bytes 12–19
├───────────────────────────────────────────────────────────────────┤
│              Padding to configured packet_size_bytes              │  bytes 20+
└───────────────────────────────────────────────────────────────────┘
```

The mirror echoes the datagram back verbatim without inspecting anything beyond
the magic number. The sender identifies which echoes correspond to which sends
via the sequence number.

### Adding a new TUI screen

1. Add a `show<ScreenName>()` method to `App` in `internal/tui/app.go`.
2. Build the widget tree using `tview` primitives.
3. Call `a.switchTo("<page-name>", rootPrimitive)` to make it active.
4. Call `a.tv.SetFocus(focusTarget)` to direct keyboard input.
5. For goroutine-driven updates, use `a.tv.QueueUpdateDraw(func() { ... })`.

### Running locally

```sh
go run .
```

For a quick two-process test on one machine, use different ports:

```
# terminal 1 — mirror
./switch-tester-5000  →  Mirroring Computer  →  port 5000

# terminal 2 — measure
./switch-tester-5000  →  Measuring Computer
  Mirror IP:   127.0.0.1
  Mirror Port: 5000
  Listen Port: 5001
```

---

## Releasing

`version.txt` is the single source of truth. The Makefile reads it at build
time and injects it via `-ldflags` so it appears in the welcome screen.

```sh
python create-release.py           # patch bump:  0.1.1 → 0.1.2
python create-release.py --minor   # minor bump:  0.1.2 → 0.2.0
python create-release.py --major   # major bump:  0.2.0 → 1.0.0
python create-release.py --dry-run # preview without making changes
```

The script:
1. Checks the working tree is clean (uncommitted changes abort)
2. Increments the requested version component, resets lower components to 0
3. Writes `version.txt`
4. Commits: `chore: release vX.Y.Z`
5. Creates an annotated tag `vX.Y.Z`
6. Pushes the commit and tag to `origin`

The GitHub Actions workflow (`.github/workflows/release.yml`) triggers on any
`v*` tag, builds all five platform binaries via `make all`, generates SHA-256
checksums, and publishes them as a GitHub Release.
